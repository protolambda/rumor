package conn_mngr

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	core "github.com/libp2p/go-libp2p-core"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"math/rand"
	"time"
)

type Score uint64
type Tag string

// Protected peers:
// - also count towards these numbers
// - never get pruned
// - may cause others to get pruned.
// - are always requested to get added, regardless of target/hi/max
type Goal struct {
	Low int
	Target int
	Hi int
	Enabled bool
}

type protectTask struct {
	id peer.ID
	protected bool
}

type ConnManager struct {
	h core.Host
	balanceBeat time.Duration
	connectionBeat time.Duration

	goal Goal

	// changeGoal asks the processor to change balance goals
	changeGoal chan Goal

	// time to leave a pool peer alone
	cooldownPeriod time.Duration

	// time to leave a connected peer alone
	gracePeriod time.Duration

	// Input, any potentially new peers should go in here.
	// Also used to refresh already known peers
	pipeline chan peer.ID

	// a peer may only be in one of the maps: connecting, adding, pruning, connected, pool

	// tracking who we are connecting to
	connecting map[peer.ID]struct{}
	// tracking who we are planning to add
	adding map[peer.ID]struct{}
	// tracking who we are planning to prune
	pruning map[peer.ID]struct{}
	// tracking who is connected
	connected map[peer.ID]struct{}
	// pool of peers we can pick from
	pool map[peer.ID]struct{}

	lastConnectionAttempt map[peer.ID]time.Time
	lastUpdate map[peer.ID]time.Time

	// protected peers will be sought after for a connection more eagerly, and will not get pruned.
	protected map[peer.ID]struct{}

	protect chan protectTask

}

func NewConnManager(h core.Host, g Goal) *ConnManager {
	return &ConnManager{
		h:                     h,
		balanceBeat:           time.Second * 60,
		connectionBeat:        time.Second * 30,
		goal:                  g,
		changeGoal:            make(chan Goal),
		cooldownPeriod:        time.Second * 20,
		gracePeriod:           time.Second * 20,
		pipeline:              make(chan peer.ID, 100),
		connecting:            make(map[peer.ID]struct{}, 100),
		adding:                make(map[peer.ID]struct{}, 100),
		pruning:               make(map[peer.ID]struct{}, 100),
		connected:             make(map[peer.ID]struct{}, 100),
		pool:                  make(map[peer.ID]struct{}, 100),
		lastConnectionAttempt: make(map[peer.ID]time.Time, 100),
		lastUpdate:            make(map[peer.ID]time.Time, 100),
		protected:             make(map[peer.ID]struct{}, 10),
		protect:               make(chan protectTask),
	}
}

type ValueFunc func(totals map[Tag]uint64, contrib map[Tag]struct{}) float64

// ChangeGoal changes the goal and blocks until the change is processed
func (c *ConnManager) ChangeGoal(g Goal) {
	c.changeGoal <- g
}

// Load the full peerstore into the pool to try and make connections with
func (c *ConnManager) Refresh() {
	for _, p := range c.h.Peerstore().Peers() {
		c.pipeline <- p
	}
}

// SharePeer makes the connection manager aware of a peer (and it may already be)
func (c *ConnManager) SharePeer(p peer.ID) {
	c.pipeline <- p
}

func (c *ConnManager) processLoop(ctx context.Context) {

	balanceTicker := time.NewTicker(c.balanceBeat)
	defer balanceTicker.Stop()
	connectionTicker := time.NewTicker(c.connectionBeat)
	defer connectionTicker.Stop()

	netw := c.h.Network()
	pStore := c.h.Peerstore()

	var seed [8]byte
	crand.Read(seed[:])
	src := rand.NewSource(int64(binary.LittleEndian.Uint64(seed[:])))
	rng := rand.New(src)

	toPrune := make([]peer.ID, 0, 1000)
	toAdd := make([]peer.ID, 0, 1000)
	preferAdd := make([]peer.ID, 0, 10)

	for {
		select {
		case <-ctx.Done():
			return
		case g := <- c.changeGoal:
			c.goal = g
		case p := <- c.protect:
			if p.protected {
				c.protected[p.id] = struct{}{}
			} else {
				delete(c.protected, p.id)
			}
		case candidate := <- c.pipeline:  // keep taking any inputs
			c.lastUpdate[candidate] = time.Now()
			switch netw.Connectedness(candidate) {
			case network.NotConnected, network.CanConnect:  // ok to pick later
				delete(c.connected, candidate)
				// if we cannot connect to it, then it does not belong in the pool
				if len(pStore.Addrs(candidate)) == 0 {
					break
				}
				if _, ok := c.adding[candidate]; ok {
					continue
				}
				if _, ok := c.pruning[candidate]; ok {
					continue
				}
				if _, ok := c.connecting[candidate]; ok {
					continue
				}
				c.pool[candidate] = struct{}{}
			case network.Connected:  // already connected, not for picking later
				c.connected[candidate] = struct{}{}
				delete(c.pool, candidate)
				delete(c.connecting, candidate)
			case network.CannotConnect:  // cannot connect, don't start picking it again, retries will do the work.
				delete(c.connected, candidate)
				delete(c.pool, candidate)
				delete(c.connecting, candidate)
			}
		case <- connectionTicker.C:
			for p := range c.pruning {
				// TODO goodbye message
				netw.ClosePeer(p)
			}
			for p := range c.adding {
				netw.DialPeer(ctx, p)
				// TODO status message
			}

			// TODO make/break connections

		case <- balanceTicker.C:  // do a heartbeat to decide how to prune/create any connections
			if !c.goal.Enabled {
				continue
			}
			now := time.Now()
			graceThreshold := now.Add(c.gracePeriod)
			cooldownThreshold := now.Add(c.cooldownPeriod)

			// reset buffers
			toPrune = toPrune[:0]
			toAdd = toAdd[:0]
			preferAdd = preferAdd[:0]

			// see what is there to prune
			for p := range c.connected {
				// Don't dare to prune protected peers
				if _, ok := c.protected[p]; ok {
					continue
				}
				// grace period: don't prune too quickly
				if last, ok := c.lastConnectionAttempt[p]; ok && graceThreshold.After(last) {
					continue
				}
				// and not if we already are
				if _, ok := c.pruning[p]; ok {
					continue
				}
				toPrune = append(toPrune, p)
			}

			// c.connecting: just don't touch ongoing connection attempts

			// see what is there to add
			for p := range c.pool {
				// cooldown period: don't re-add too quickly
				if last, ok := c.lastConnectionAttempt[p]; ok && cooldownThreshold.After(last) {
					continue
				}
				if _, ok := c.protected[p]; ok {
					preferAdd = append(preferAdd, p)  // we would like to connect to the protected peer
					continue
				}
				toAdd = append(toAdd, p)
			}

			// add the peers we prefer to add over any of the others
			for _, p := range preferAdd {
				c.adding[p] = struct{}{}
				delete(c.pool, p)
			}

			// try to get back to low water
			current := len(c.connected) + len(c.connecting) + len(preferAdd)
			if current < c.goal.Low && current < c.goal.Target {
				// randomize to not get stuck to picking the same peers
				rng.Shuffle(len(toAdd), func(i, j int) {
					toAdd[i], toAdd[j] = toAdd[j], toAdd[i]  // TODO: bias towards peer potential value
				})
				wanted := c.goal.Target-current
				if wanted < len(toAdd) {
					toAdd = toAdd[:wanted]
				}
				// ask for connections
				for _, p := range toAdd {
					c.adding[p] = struct{}{}
					delete(c.pool, p)
				}
			} else if current > c.goal.Hi && current > c.goal.Target {
				rng.Shuffle(len(toPrune), func(i, j int) {
					toPrune[i], toPrune[j] = toPrune[j], toPrune[i]  // TODO: bias towards lower peer scores
				})
				unwanted := c.goal.Target-current
				if unwanted < len(toPrune) {
					toPrune = toPrune[:unwanted]
				}
				// ask for pruning
				for _, p := range toPrune {
					c.pruning[p] = struct{}{}
					delete(c.pool, p)
				}
			}
		}
	}
}

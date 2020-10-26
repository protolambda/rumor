package peer

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/zrnt/eth2/beacon"
	"reflect"
	"sync"
	"time"
)

type PeerConnectAllCmd struct {
	*base.Base
	Store      track.ExtendedPeerstore
	Timeout    time.Duration `ask:"--timeout" help:"connection timeout, 0 to disable"`
	Rescan     time.Duration `ask:"--rescan" help:"rescan the peerscore for new peers to connect with this given interval"`
	MaxRetries uint64        `ask:"--max-retries" help:"how many connection attempts until the peer is banned"`
	Workers    uint64        `ask:"--workers" help:"how many parallel routines should be attempting connections"`
	MaxPeers   uint64        `ask:"--max-peers" help:"max amount of peers, pause auto-connecting when above this"`

	FilterDigest beacon.ForkDigest `ask:"--filter-digest" help:"Only connect when the peer is known to have the given fork digest in ENR. Or connect to any if not specified."`
	Filtering    bool              `changed:"filter-digest"`
}

func (c *PeerConnectAllCmd) Default() {
	c.Timeout = 10 * time.Second
	c.Rescan = 1 * time.Minute
	c.MaxRetries = 5
	c.Workers = 1
	c.MaxPeers = 200
}

func (c *PeerConnectAllCmd) Help() string {
	return "Auto-connect to peers in the peerstore."
}

type consumePeerFn func(ctx context.Context) (p peer.ID, priority uint64, ok bool)

// priorityPeerConsumer builds a function to consume a peer ID from the top-priority channel first,
// then select-defaults to selecting the top 2, then top 3, etc.
// The prioritized channels are sorted by decreasing priority.
// Cannot prioritize more than 65534 cases due to golang select limits.
// The returned function is safe to call concurrently.
func priorityPeerConsumer(prioritized []chan peer.ID) consumePeerFn {
	if len(prioritized) == 0 {
		return func(ctx context.Context) (res peer.ID, priority uint64, ok bool) {
			return "", 0, false
		}
	}
	if len(prioritized) > 65534 {
		panic("too many channels to prioritize")
	}

	selectCases := make([]reflect.SelectCase, 0, len(prioritized))
	for _, ch := range prioritized {
		selectCases = append(selectCases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ch),
		})
	}

	// essentially build a big select statement to read channels by priority
	// ctx/0/default(ctx/0/1/default(ctx/0/1/2/default(0/1/2/3/default(...))))
	withBackup := func(chs []reflect.SelectCase, forPriority uint64, fn consumePeerFn) consumePeerFn {
		return func(ctx context.Context) (res peer.ID, priority uint64, ok bool) {
			options := make([]reflect.SelectCase, len(chs)+1, len(chs)+2)
			options[0] = reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(ctx.Done()),
			}
			copy(options[1:], chs)
			if fn != nil {
				options = append(options, reflect.SelectCase{
					Dir: reflect.SelectDefault,
				})
			}
			chosen, val, ok := reflect.Select(options)
			if chosen == len(chs)+1 { // the select-default case
				return fn(ctx)
			}
			if !ok {
				return "", 0, false
			}
			return val.Interface().(peer.ID), forPriority, true
		}
	}

	var next consumePeerFn = nil
	for i := len(prioritized) - 1; i >= 0; i-- {
		next = withBackup(selectCases[:i+1], uint64(i), next)
	}
	return next
}

// priorityPeerScheduler returns a function to schedule peer IDs with a given priority.
// If the priority channel is full, it cascades to scheduling as a lower priority event.
// If it could not be scheduled, it returns ok=false.
// The returned function is safe to call concurrently.
func priorityPeerScheduler(prioritized []chan peer.ID) func(p peer.ID, priority int) (givenPriority int, ok bool) {
	return func(p peer.ID, priority int) (givenPriority int, ok bool) {
		for i := priority; i < len(prioritized); i++ {
			select {
			case prioritized[i] <- p:
				return i, true
			default:
				continue
			}
		}
		return 0, false
	}
}

func closeSchedulingChannels(prioritized []chan peer.ID) {
	for i := 0; i < len(prioritized); i++ {
		// future schedules will block, and resort to lower priority channel, but none will be left, so they exit.
		ch := prioritized[i]
		prioritized[i] = nil
		close(ch)
	}
}

func (c *PeerConnectAllCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	if c.Workers > 200 {
		return fmt.Errorf("excessive worker count: %d", c.Workers)
	}

	bgCtx, bgCancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.run(bgCtx, h)
		close(done)
	}()

	c.Control.RegisterStop(func(ctx context.Context) error {
		bgCancel()
		c.Log.Infof("Stopped auto-connecting")
		<-done
		return nil
	})
	return nil
}

func (c *PeerConnectAllCmd) run(ctx context.Context, h host.Host) {
	var peerAttemptLock sync.Mutex
	peerAttempts := make(map[peer.ID]uint64)
	max := c.MaxRetries
	priorityFloor := uint64(10) // after this retries, they all have the same priority
	if max > priorityFloor {
		max = priorityFloor
	}
	byAttempts := make([]chan peer.ID, max)
	for i := range byAttempts {
		byAttempts[i] = make(chan peer.ID, 100)
	}
	defer closeSchedulingChannels(byAttempts)
	next := priorityPeerConsumer(byAttempts)
	schedule := priorityPeerScheduler(byAttempts)

	var workersGroup sync.WaitGroup
	worker := func(i uint64) {
		defer workersGroup.Done()
		log := c.Log.WithField("worker", i)

		log.Info("started worker")
		for {
			p, pi, ok := next(ctx)
			if !ok { // if background context closes, the workers will stop, and free up the workersGroup
				break
			}
			// libp2p complains if we put multi-addresses that include the peer ID into the Addrs list.
			addrs := c.Store.Addrs(p)
			addrInfo := peer.AddrInfo{
				ID:    p,
				Addrs: make([]ma.Multiaddr, 0, len(addrs)),
			}
			for _, m := range addrs {
				transport, _ := peer.SplitAddr(m)
				if transport == nil {
					continue
				}
				addrInfo.Addrs = append(addrInfo.Addrs, transport)
			}
			ctx, _ := context.WithTimeout(ctx, c.Timeout)
			attemptLog := log.WithField("peer_id", p)
			attemptLog.WithField("addrs", addrInfo.Addrs).Debug("attempting connection to peer")
			// Slight chance we're already connected due to duplicate scheduling, but that's ok, nothing happens.
			if err := h.Connect(ctx, addrInfo); err != nil {
				// increment attempts
				peerAttemptLock.Lock()
				// default value is 0, that's ok
				peerAttempts[p] += 1
				a := peerAttempts[p]
				peerAttemptLock.Unlock()
				if a <= c.MaxRetries {
					if a >= priorityFloor {
						a = priorityFloor
					}
					attemptLog.WithError(err).WithField("attempts", a).
						Debug("failed connection attempt, scheduling retry...")
					go func() {
						// The lower the priority, the longer we wait.
						time.Sleep(time.Second * 5 * time.Duration(pi))
						if _, ok := schedule(p, int(a)); !ok {
							attemptLog.Debug("failed to reschedule, dropping attempt")
						}
					}()
				} else {
					attemptLog.WithError(err).WithField("attempts", a).
						Debug("failed connection attempt, reached maximum, no retry")
				}
			} else {
				log.WithField("peer_id", p).Debug("successful connection made")
				// reset attempts
				peerAttemptLock.Lock()
				peerAttempts[p] = 0
				peerAttemptLock.Unlock()
			}
		}

		log.Debug("stopped connection worker")
	}

	workersGroup.Add(int(c.Workers))
	for w := uint64(0); w < c.Workers; w++ {
		go worker(w)
	}

	scan := func() {
		storedPeers := c.Store.PeersWithAddrs()

		var schedules []peer.ID
		peerAttemptLock.Lock()
		for _, p := range storedPeers {
			// Check if it didn't fail before (unknown peer or success last time)
			if v, ok := peerAttempts[p]; !ok || v == 0 {
				if c.Filtering { // optionally filter by fork-digest
					enr := c.Store.LatestENR(p)
					if enr == nil {
						continue
					}
					eth2Data, ok, err := addrutil.ParseEnrEth2Data(enr)
					if err != nil || !ok {
						continue
					}
					if eth2Data.ForkDigest != c.FilterDigest {
						continue
					}
				}
				// Check if we're connected already
				if status := h.Network().Connectedness(p); status != network.Connected {
					schedules = append(schedules, p)
				}
			}
		}
		peerAttemptLock.Unlock()

		c.Log.Infof("scanned peerstore, found %d peers to schedule", len(schedules))
		count := len(h.Network().Peers())
		if est := uint64(count + len(schedules)); est > c.MaxPeers {
			if uint64(count) >= c.MaxPeers {
				schedules = nil
			} else {
				schedules = schedules[:c.MaxPeers-uint64(count)]
			}
			c.Log.Warnf("too many peers, adjusted scheduled peers to %d", len(schedules))
		}
		for _, p := range schedules {
			// Schedule the peer with good priority
			if _, ok := schedule(p, 0); !ok {
				// oh no, too many peers scheduled. Skip this fill round.
				c.Log.Warn("Scheduled too many peer connections, aborting")
				break
			}
		}
	}

	// And add the producer
	workersGroup.Add(1)

	go func() {
		defer workersGroup.Done()

		scanTicker := time.NewTicker(c.Rescan)
		defer scanTicker.Stop()

		// initial scan
		scan()

		for {
			select {
			case <-scanTicker.C:
				scan()
			case <-ctx.Done():
				c.Log.Debug("stopped scanning peerstore for connect-all")
				return
			}
		}
	}()

	// Wait for everything to shut down, only then the deferred channel closes will run:
	// to be safe from writing to closed channels.
	workersGroup.Wait()
}

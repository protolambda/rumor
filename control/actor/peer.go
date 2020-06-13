package actor

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/sirupsen/logrus"
	"strings"
	"sync"
	"time"
)

type PeerCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
}

func (c *PeerCmd) Help() string {
	return "Manage the libp2p peerstore"
}

func (c *PeerCmd) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) {
	if len(args) == 0 {
		return nil, nil, errors.New("no subcommand specified")
	}
	switch args[0] {
	case "start":
		cmd = DefaultHostStartCmd(c.Actor, c.log)
	// TODO
	default:
		return nil, args, fmt.Errorf("unrecognized command: %v", args)
	}
	return cmd, args[1:], nil
}

func (c *PeerCmd) PeerList() *PeerListCmd {
	return &PeerListCmd{
		PeerCmd:       c,
		Which:         "connected",
		ListLatency:   false,
		ListProtocols: false,
		ListAddrs:     true,
		ListStatus:    false,
		ListMetadata:  false,
		ListClaimSeq:  false,
	}
}

func (c *PeerCmd) Trim() *PeerTrimCmd {
	return &PeerTrimCmd{
		PeerCmd: c,
		Timeout: time.Second * 2,
	}
}

type PeerListCmd struct {
	*PeerCmd `ask:"-"`

	Which string `ask:"[which]" help:"Which peers to list, possible values: 'all', 'connected'."`

	ListLatency   bool `ask:"--latency" help:"list peer latency"`
	ListProtocols bool `ask:"--protocols" help:"list peer protocols"`
	ListAddrs     bool `ask:"--addrs" help:"list peer addrs"`
	ListStatus    bool `ask:"--status" help:"list peer status"`
	ListMetadata  bool `ask:"--metadata" help:"list peer metadata"`
	ListClaimSeq  bool `ask:"--claimseq" help:"list peer claimed metadata seq nr"`
}

func (c *PeerListCmd) Help() string {
	return "Stop the host node."
}

func (c *PeerListCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		args = append(args, "connected")
	}
	var peers []peer.ID
	switch args[0] {
	case "all":
		peers = h.Peerstore().Peers()
	case "connected":
		peers = h.Network().Peers()
	default:
		return fmt.Errorf("invalid peer type: %s", args[0])
	}
	store := h.Peerstore()
	peerData := make(map[peer.ID]map[string]interface{})
	for _, p := range peers {
		v := make(map[string]interface{})
		if c.ListAddrs {
			v["addrs"] = store.PeerInfo(p).Addrs
		}
		// TODO: add dv5 node ID
		if c.ListLatency {
			v["latency"] = store.LatencyEWMA(p).Seconds() // A float, ok for json
		}
		if c.ListProtocols {
			protocols, err := store.GetProtocols(p)
			if err != nil {
				v["protocols"] = protocols
			}
		}
		if c.ListStatus || c.ListMetadata || c.ListClaimSeq {
			pInfoData, ok := c.GlobalPeerInfos.Find(p)
			if ok {
				if c.ListStatus {
					v["status"] = pInfoData.Status()
				}
				if c.ListMetadata {
					v["metadata"] = pInfoData.Metadata()
				}
				if c.ListClaimSeq {
					v["metadata"] = pInfoData.ClaimedSeq()
				}
			}
		}
		peerData[p] = v
	}
	c.log.WithField("peers", peerData).Infof("%d peers", len(peers))
	return nil
}

type PeerTrimCmd struct {
	*PeerCmd `ask:"-"`
	Timeout  time.Duration `ask:"[which]" help:"Timeout for trimming."`
}

func (c *PeerTrimCmd) Help() string {
	return "Trim peers, with timeout."
}

func (c *PeerTrimCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	trimCtx, _ := context.WithTimeout(ctx, c.Timeout)
	h.ConnManager().TrimOpenConns(trimCtx)
	return nil
}

type PeerConnectCmd struct {
	*PeerCmd `ask:"-"`
	Timeout  time.Duration    `ask:"--timeout" help:"connection timeout, 0 to disable"`
	Addr     FlexibleAddrFlag `ask:"<addr>" help:"ENR, enode or multi address to connect to"`
	Tag      string           `ask:"[tag]" help:"Optionally tag the peer upon connection, e.g. tag 'bootnode'"`
}

func (c *PeerConnectCmd) Help() string {
	return "Connect to peer."
}

func (c *PeerConnectCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	addrInfo, err := peer.AddrInfoFromP2pAddr(c.Addr.MultiAddr)
	if err != nil {
		return err
	}
	if c.Timeout != 0 {
		ctx, _ = context.WithTimeout(ctx, c.Timeout)
	}
	if err := h.Connect(ctx, *addrInfo); err != nil {
		return err
	}
	c.log.WithField("peer_id", addrInfo.ID.Pretty()).Infof("connected to peer")
	if c.Tag != "" {
		h.ConnManager().Protect(addrInfo.ID, c.Tag)
		c.log.Infof("tagged peer %s as %s", addrInfo.ID.Pretty(), c.Tag)
	}
	return nil
}

type PeerDisconnectCmd struct {
	*PeerCmd `ask:"-"`
	PeerID   PeerIDFlag `ask:"<peer-id>" help:"The peer to close all connections of"`
}

func (c *PeerDisconnectCmd) Help() string {
	return "Close all open connections with the given peer"
}

func (c *PeerDisconnectCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	conns := h.Network().ConnsToPeer(c.PeerID.PeerID)
	for _, conn := range conns {
		if err := conn.Close(); err != nil {
			c.log.Infof("error during disconnect of peer %s (%s)",
				c.PeerID.PeerID.Pretty(), conn.RemoteMultiaddr().String())
		}
	}
	c.log.Infof("disconnected peer %s", c.PeerID.PeerID.Pretty())
	return nil
}

type PeerProtectCmd struct {
	*PeerCmd `ask:"-"`
	PeerID   PeerIDFlag `ask:"<peer-id>" help:"The peer to protect with a tag"`
	Tag      string     `ask:"<tag>" help:"Tag to give to the peer"`
}

func (c *PeerProtectCmd) Help() string {
	return "Protect a peer by giving it a tag"
}

func (c *PeerProtectCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	h.ConnManager().Protect(c.PeerID.PeerID, c.Tag)
	c.log.Infof("protected peer %s as %s", c.PeerID.PeerID.Pretty(), c.Tag)
	return nil
}

type PeerUnprotectCmd struct {
	*PeerCmd `ask:"-"`
	PeerID   PeerIDFlag `ask:"<peer-id>" help:"The peer to un-protect with a tag"`
	Tag      string     `ask:"<tag>" help:"Tag to remove from the peer"`
}

func (c *PeerUnprotectCmd) Help() string {
	return "Unprotect a peer by removing a tag"
}

func (c *PeerUnprotectCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	h.ConnManager().Unprotect(c.PeerID.PeerID, c.Tag)
	c.log.Infof("un-protected peer %s as %s", c.PeerID.PeerID.Pretty(), c.Tag)
	return nil
}

type PeerAddrsCmd struct {
	*PeerCmd `ask:"-"`
	PeerID   PeerIDFlag `ask:"[peer-id]" help:"The peer to view addresses of, or local peer if omitted."`
}

func (c *PeerAddrsCmd) Help() string {
	return "View known addresses of [peerID]. Defaults to local addresses if no peer id is specified."
}

func (c *PeerAddrsCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	if c.PeerID.PeerID == "" {
		addrs := h.Addrs()
		c.log.WithField("addrs", addrs).Infof("host addrs")
	} else {
		addrs := h.Peerstore().Addrs(c.PeerID.PeerID)
		c.log.WithField("addrs", addrs).Infof("addrs for peer %s", c.PeerID.PeerID.Pretty())
	}
	return nil
}

func parseRoot(v string) ([32]byte, error) {
	if v == "0" {
		return [32]byte{}, nil
	}
	if strings.HasPrefix(v, "0x") {
		v = v[2:]
	}
	if len(v) != 64 {
		return [32]byte{}, fmt.Errorf("provided root has length %d, expected 64 hex characters (ignoring optional 0x prefix)", len(v))
	}
	var out [32]byte
	_, err := hex.Decode(out[:], []byte(v))
	return out, err
}

func parseForkVersion(v string) ([4]byte, error) {
	if strings.HasPrefix(v, "0x") {
		v = v[2:]
	}
	if len(v) != 8 {
		return [4]byte{}, fmt.Errorf("provided fork version has length %d, expected 8 hex characters (ignoring optional 0x prefix)", len(v))
	}
	var out [4]byte
	_, err := hex.Decode(out[:], []byte(v))
	return out, err
}

type PeerStatusState struct {
	Following bool
	Local     methods.Status
}

type PeerStatusCmd struct {
	*PeerCmd
}

func (c *PeerStatusCmd) Help() string {
	return "Manage and track peer status"
}

func (c *PeerStatusCmd) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) {
	if len(args) == 0 {
		return nil, nil, errors.New("no subcommand specified")
	}
	switch args[0] {
	case "req":
		cmd = &PeerStatusReqCmd{
			PeerStatusCmd: c,
			Timeout:       10 * time.Second,
			Compression:   CompressionFlag{Compression: reqresp.SnappyCompression{}},
		}
	// TODO
	default:
		return nil, args, fmt.Errorf("unrecognized command: %v", args)
	}
	return cmd, args[1:], nil
}

func (c *PeerStatusCmd) fetch(ctx context.Context, timeout time.Duration, peerID peer.ID, comp reqresp.Compression) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	sFn := reqresp.NewStreamFn(h.NewStream)
	reqCtx := ctx
	if timeout != 0 {
		reqCtx, _ = context.WithTimeout(reqCtx, timeout)
	}
	m := methods.StatusRPCv1

	var reqStatus methods.Status
	if c.PeerStatusState.Following {
		// TODO get status from chain
	} else {
		reqStatus = c.PeerStatusState.Local
	}
	return m.RunRequest(reqCtx, sFn, peerID, comp,
		reqresp.RequestSSZInput{Obj: &reqStatus}, 1,
		func(chunk reqresp.ChunkedResponseHandler) error {
			resultCode := chunk.ResultCode()
			f := map[string]interface{}{
				"from":        peerID.String(),
				"result_code": resultCode,
			}
			switch resultCode {
			case reqresp.ServerErrCode, reqresp.InvalidReqCode:
				msg, err := chunk.ReadErrMsg()
				if err != nil {
					return err
				}
				f["msg"] = msg
			case reqresp.SuccessCode:
				var data methods.Status
				if err := chunk.ReadObj(&data); err != nil {
					return err
				}
				f["data"] = data
				inf, _ := c.GlobalPeerInfos.Find(peerID)
				inf.RegisterStatus(data)
			}
			c.log.WithFields(f).Debug("got status response")
			return nil
		})
}

func (c *PeerStatusCmd) Req() *PeerStatusReqCmd {
	return &PeerStatusReqCmd{
		PeerStatusCmd: c,
		Timeout:       10 * time.Second,
		Compression:   CompressionFlag{Compression: reqresp.SnappyCompression{}},
	}
}

type PeerStatusReqCmd struct {
	*PeerStatusCmd `ask:"-"`
	Timeout        time.Duration   `ask:"--timeout" help:"request timeout, 0 to disable"`
	Compression    CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
	PeerID         PeerIDFlag      `ask:"<peer-id>" help:"Peer to fetch status from."`
}

func (c *PeerStatusReqCmd) Help() string {
	return "Fetch status of connected peer."
}

func (c *PeerStatusReqCmd) Run(ctx context.Context, args ...string) error {
	return c.fetch(ctx, c.Timeout, c.PeerID.PeerID, c.Compression.Compression)
}

type PeerStatusPollCmd struct {
	*PeerStatusCmd `ask:"-"`
	Timeout        time.Duration   `ask:"--timeout" help:"request timeout, 0 to disable"`
	Interval       time.Duration   `ask:"--interval" help:"interval to request status of peers on"`
	Compression    CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
}

func (c *PeerStatusPollCmd) Help() string {
	return "Fetch status of all connected peers, repeatedly on the given interval."
}

func (c *PeerStatusPollCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	for {
		start := time.Now()
		var wg sync.WaitGroup
		for _, p := range h.Network().Peers() {
			// TODO: maybe filter peers that cannot answer status requests?
			wg.Add(1)
			go func(peerID peer.ID) {
				if err := c.fetch(ctx, c.Timeout, peerID, c.Compression.Compression); err != nil {
					c.log.Warn(err)
				}
				wg.Done()
			}(p)
		}
		wg.Wait()
		pollStepDuration := time.Since(start)
		if pollStepDuration < c.Interval {
			time.Sleep(c.Interval - pollStepDuration)
		}
		select {
		case <-ctx.Done():
			return nil
		default:
			// next interval
		}
	}
}

type PeerStatusGetCmd struct {
	*PeerStatusCmd `ask:"-"`
}

func (c *PeerStatusGetCmd) Help() string {
	return "Get current status and if following the chain or not."
}

func (c *PeerStatusGetCmd) Run(ctx context.Context, args ...string) error {
	c.log.WithFields(logrus.Fields{
		"following": c.PeerStatusState.Following,
		"status":    c.PeerStatusState.Local,
	}).Info("Status settings")
	return nil
}

type PeerStatusSetCmd struct {
	*PeerStatusCmd `ask:"-"`
	ForkVersion    beacon.Version `ask:"--fork-version"`
	HeadRoot       beacon.Root    `ask:"--head-root"`
	HeadSlot       beacon.Slot    `ask:"--head-slot"`
	FinalizedRoot  beacon.Root    `ask:"--finalized-root"`
	FinalizedEpoch beacon.Epoch   `ask:"--finalized-epoch"`
	Following      bool           `ask:"--following" help:"If the status should automatically follow the current chain (if any)"`
}

func (c *PeerStatusSetCmd) Help() string {
	return "Set (a part of) the current status and if following the chain or not."
}

func (c *PeerStatusSetCmd) Run(ctx context.Context, args ...string) error {
	// TODO: only change each of these if they were modified
	c.PeerStatusState.Local.HeadForkVersion = c.ForkVersion
	c.PeerStatusState.Local.HeadRoot = c.HeadRoot
	c.PeerStatusState.Local.HeadSlot = c.HeadSlot
	c.PeerStatusState.Local.FinalizedEpoch = c.FinalizedEpoch
	c.PeerStatusState.Local.FinalizedRoot = c.FinalizedRoot
	c.PeerStatusState.Following = c.Following

	c.log.WithFields(logrus.Fields{
		"following": c.PeerStatusState.Following,
		"status":    c.PeerStatusState.Local,
	}).Info("Status settings")
	return nil
}

type PeerStatusServeCmd struct {
	*PeerStatusCmd `ask:"-"`

	// TODO set default
	Timeout     time.Duration   `ask:"--timeout" help:"Apply timeout of n milliseconds to each stream (complete request <> response time). 0 to Disable timeout"`
	Compression CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
}

func (c *PeerStatusServeCmd) Help() string {
	return "Serve incoming status requests"
}

func (c *PeerStatusServeCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	sCtxFn := func() context.Context {
		if c.Timeout == 0 {
			return ctx
		}
		reqCtx, _ := context.WithTimeout(ctx, c.Timeout)
		return reqCtx
	}
	comp := c.Compression.Compression
	listenReq := func(ctx context.Context, peerId peer.ID, handler reqresp.ChunkedRequestHandler) {
		f := map[string]interface{}{
			"from": peerId.String(),
		}
		var reqStatus methods.Status
		err := handler.ReadRequest(&reqStatus)
		if err != nil {
			f["input_err"] = err.Error()
			_ = handler.WriteErrorChunk(reqresp.InvalidReqCode, "could not parse status request")
			c.log.WithFields(f).Warnf("failed to read status request: %v", err)
		} else {
			f["data"] = reqStatus
			inf, _ := c.GlobalPeerInfos.Find(peerId)
			inf.RegisterStatus(reqStatus)

			var resp methods.Status
			if c.PeerStatusState.Following {
				// TODO
			} else {
				resp = c.PeerStatusState.Local
			}
			if err := handler.WriteResponseChunk(reqresp.SuccessCode, &resp); err != nil {
				c.log.WithFields(f).Warnf("failed to respond to status request: %v", err)
			} else {
				c.log.WithFields(f).Warnf("handled status request: %v", err)
			}
		}
	}
	m := methods.StatusRPCv1
	streamHandler := m.MakeStreamHandler(sCtxFn, comp, listenReq)
	prot := m.Protocol
	if comp != nil {
		prot += protocol.ID("_" + comp.Name())
	}
	h.SetStreamHandler(prot, streamHandler)
	c.log.WithField("started", true).Infof("Opened listener")
	<-ctx.Done()
	return nil
}

type PeerMetadataState struct {
	Following bool
	Local     methods.MetaData
}

type PeerMetadataCmd struct {
	*PeerCmd
}

func (c *PeerMetadataCmd) Help() string {
	return "Manage and track peer metadata"
}

func (c *PeerMetadataCmd) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) {
	/* TODO
		  ping <peer id>  --update # request peer for pong, update metadata maybe
		  pong --update   # serve others with pongs, and if ping is new enough, request them for metadata if --update=true

		  fetch <peer id>  # get metadata of peer

		  poll <interval>  # poll connected peers for metadata by pinging them on interval

		  get
		  set --follow

		  serve   # serve meta data
	actors
	*/
	return nil, nil, errors.New("not implemented yet")
}

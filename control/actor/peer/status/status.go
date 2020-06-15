package status

import (
	"fmt"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

type PeerStatusCmd struct {
	*base.Base
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
			c.Log.WithFields(f).Debug("got status response")
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
	*base.Base
	Timeout        time.Duration   `ask:"--timeout" help:"request timeout, 0 to disable"`
	Compression    CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
	PeerID         flags.PeerIDFlag      `ask:"<peer-id>" help:"Peer to fetch status from."`
}

func (c *PeerStatusReqCmd) Help() string {
	return "Fetch status of connected peer."
}

func (c *PeerStatusReqCmd) Run(ctx context.Context, args ...string) error {
	return c.fetch(ctx, c.Timeout, c.PeerID.PeerID, c.Compression.Compression)
}

type PeerStatusPollCmd struct {
	*base.Base
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
					c.Log.Warn(err)
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
	*base.Base
}

func (c *PeerStatusGetCmd) Help() string {
	return "Get current status and if following the chain or not."
}

func (c *PeerStatusGetCmd) Run(ctx context.Context, args ...string) error {
	c.Log.WithFields(logrus.Fields{
		"following": c.PeerStatusState.Following,
		"status":    c.PeerStatusState.Local,
	}).Info("Status settings")
	return nil
}

type PeerStatusSetCmd struct {
	*base.Base
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

	c.Log.WithFields(logrus.Fields{
		"following": c.PeerStatusState.Following,
		"status":    c.PeerStatusState.Local,
	}).Info("Status settings")
	return nil
}

type PeerStatusServeCmd struct {
	*base.Base

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
			c.Log.WithFields(f).Warnf("failed to read status request: %v", err)
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
				c.Log.WithFields(f).Warnf("failed to respond to status request: %v", err)
			} else {
				c.Log.WithFields(f).Warnf("handled status request: %v", err)
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
	c.Log.WithField("started", true).Infof("Opened listener")
	<-ctx.Done()
	return nil
}

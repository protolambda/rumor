package status

import (
	"context"
	"errors"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
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
			Compression:   flags.CompressionFlag{Compression: reqresp.SnappyCompression{}},
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
		Compression:   flags.CompressionFlag{Compression: reqresp.SnappyCompression{}},
	}
}




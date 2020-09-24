package status

import (
	"context"
	"errors"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/zrnt/eth2/beacon"
)

type PeerStatusState struct {
	Following bool
	Local     beacon.Status
}

type PeerStatusCmd struct {
	*base.Base
	*PeerStatusState
	Book track.StatusBook
}

func (c *PeerStatusCmd) Help() string {
	return "Manage and track peer status"
}

func (c *PeerStatusCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "get":
		cmd = &PeerStatusGetCmd{Base: c.Base, PeerStatusState: c.PeerStatusState}
	case "set":
		cmd = &PeerStatusSetCmd{Base: c.Base, PeerStatusState: c.PeerStatusState}
	case "req":
		cmd = &PeerStatusReqCmd{Base: c.Base, PeerStatusState: c.PeerStatusState, Book: c.Book}
	case "poll":
		cmd = &PeerStatusPollCmd{Base: c.Base, PeerStatusState: c.PeerStatusState, Book: c.Book}
	case "serve":
		cmd = &PeerStatusServeCmd{Base: c.Base, PeerStatusState: c.PeerStatusState, Book: c.Book}
	case "follow":
		cmd = &PeerStatusFollowCmd{Base: c.Base, PeerStatusState: c.PeerStatusState}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *PeerStatusState) Routes() []string {
	return []string{"get", "set", "req", "poll", "serve", "follow"}
}

func (c *PeerStatusState) fetch(book track.StatusBook, sFn reqresp.NewStreamFn, ctx context.Context, peerID peer.ID, comp reqresp.Compression) (
	resCode reqresp.ResponseCode, errMsg string, data *beacon.Status, err error) {
	resCode = reqresp.ServerErrCode // error by default
	err = methods.StatusRPCv1.RunRequest(ctx, sFn, peerID, comp,
		reqresp.RequestSSZInput{Obj: &c.Local}, 1,
		func() error {
			return nil
		},
		func(chunk reqresp.ChunkedResponseHandler) error {
			resCode = chunk.ResultCode()
			switch resCode {
			case reqresp.ServerErrCode, reqresp.InvalidReqCode:
				msg, err := chunk.ReadErrMsg()
				if err != nil {
					return err
				}
				errMsg = msg
			case reqresp.SuccessCode:
				var stat beacon.Status
				if err := chunk.ReadObj(&stat); err != nil {
					return err
				}
				data = &stat
				book.RegisterStatus(peerID, stat)
			default:
				return errors.New("unexpected result code")
			}
			return nil
		})
	return
}

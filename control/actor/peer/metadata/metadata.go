package metadata

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
)

type OnMetadata func(peerID peer.ID, status *methods.MetaData)

// Returns true if the peer has not been seen, or if the seq nr is newer than we currently know for the peer
type IsUnseen func(peerID peer.ID, seqNr methods.SeqNr) bool

type PeerMetadataState struct {
	Following bool
	Local     methods.MetaData
	OnMetadata
	IsUnseen
}

type PeerMetadataCmd struct {
	*base.Base
	*PeerMetadataState
}

func (c *PeerMetadataCmd) Help() string {
	return "Manage and track peer metadata"
}

	/* TODO
		  ping <peer id>  --update # request peer for pong, update metadata maybe
		  pong --update   # serve others with pongs, and if ping is new enough, request them for metadata if --update=true

		  fetch <peer id>  # get metadata of peer

		  poll <interval>  # poll connected peers for metadata by pinging them on interval

		  get
		  set
		  follow

		  serve   # serve meta data
	*/

func (c *PeerMetadataCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "ping":
		cmd = &PeerMetadataPingCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "pong":
		cmd = &PeerMetadataPongCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "get":
		cmd = &PeerMetadataGetCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "set":
		cmd = &PeerMetadataSetCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "req":
		cmd = &PeerMetadataReqCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "poll":
		cmd = &PeerMetadataPollCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "serve":
		cmd = &PeerMetadataServeCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "follow":
		cmd = &PeerMetadataFollowCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *PeerMetadataCmd) Routes() []string {
	return []string{"ping", "pong", "get", "set", "req", "poll", "serve", "follow"}
}

func (c *PeerMetadataState) fetch(sFn reqresp.NewStreamFn, ctx context.Context, peerID peer.ID, comp reqresp.Compression) (
	resCode reqresp.ResponseCode, errMsg string, data *methods.MetaData, err error) {

	err = methods.MetaDataRPCv1.RunRequest(ctx, sFn, peerID, comp, reqresp.RequestSSZInput{Obj: nil}, 1,
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
				var meta methods.MetaData
				if err := chunk.ReadObj(&meta); err != nil {
					return err
				}
				data = &meta
				c.OnMetadata(peerID, &meta)
			}
			return nil
		})
	return
}

func (c *PeerMetadataState) ping(sFn reqresp.NewStreamFn, ctx context.Context, peerID peer.ID, comp reqresp.Compression) (
	resCode reqresp.ResponseCode, errMsg string, data methods.Pong, err error) {

	p := methods.Ping(c.Local.SeqNumber)
	err = methods.PingRPCv1.RunRequest(ctx, sFn, peerID, comp, reqresp.RequestSSZInput{Obj: &p}, 1,
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
				var pong methods.Pong
				if err := chunk.ReadObj(&pong); err != nil {
					return err
				}
				data = pong
			}
			return nil
		})
	return
}

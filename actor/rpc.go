package actor

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/rumor/rpc/methods"
	"github.com/protolambda/rumor/rpc/reqresp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"strconv"
	"strings"
	"time"
)

type RPCState struct {
	CurrentStatus methods.Status
}

func (r *Actor) InitRpcCmd(log logrus.FieldLogger, state *RPCState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rpc",
		Short: "Manage Eth2 RPC",
	}

	readOptionalComp := func(cmd *cobra.Command) (reqresp.Compression, error) {
		if compStr, err := cmd.Flags().GetString("compression"); err != nil {
			return nil, err
		} else {
			switch compStr {
			case "none", "", "false":
				// no compression
				return nil, nil
			case "snappy":
				return reqresp.SnappyCompression{}, nil
			default:
				return nil, fmt.Errorf("cannot recognize compression '%s'", compStr)
			}
		}
	}
	// TODO: stop responses command

	makeReqCmd := func(cmd *cobra.Command,
		rpcMethod func(cmd *cobra.Command) *reqresp.RPCMethod,
		mkReq func(cmd *cobra.Command, args []string) (reqresp.Request, error),
		onResp func(peerID peer.ID, chunkIndex uint64, readChunk func(dest interface{}) error) error,
		onClose func(peerID peer.ID),
	) *cobra.Command {
		cmd.Flags().String("compression", "none", "Optional compression. Try 'snappy' for streaming-snappy")
		cmd.Run = func(cmd *cobra.Command, args []string) {
			if r.NoHost(log) {
				return
			}
			sFn := reqresp.NewStreamFn(func(ctx context.Context, peerId peer.ID, protocolId protocol.ID) (network.Stream, error) {
				return r.P2PHost.NewStream(ctx, peerId, protocolId)
			}).WithTimeout(time.Second * 10)
			ctx, _ := context.WithTimeout(r.Ctx, time.Second*10) // TODO add timeout option
			peerID, err := peer.Decode(args[0])
			if err != nil {
				log.Error(err)
				return
			}
			comp, err := readOptionalComp(cmd)
			if err != nil {
				log.Error(err)
				return
			}
			req, err := mkReq(cmd, args)
			if err != nil {
				log.Error(err)
				return
			}
			m := rpcMethod(cmd)
			lastRespChunkIndex := int64(-1)
			if err := m.RunRequest(ctx, sFn, peerID, comp, req,
				func(chunkIndex uint64, readChunk func(dest interface{}) error) error {
					log.Debugf("Received response chunk %d from peer %s", chunkIndex, peerID.Pretty())
					lastRespChunkIndex = int64(chunkIndex)
					return onResp(peerID, chunkIndex, readChunk)
				}, func(chunkIndex uint64, msg string) error {
					log.Errorf("request (protocol %s) to %s was turned down at chunk %d because of INVALID INPUT. Error message from server: %s", m.Protocol, peerID, chunkIndex, msg)
					return nil
				}, func(chunkIndex uint64, msg string) error {
					log.Errorf("request (protocol %s) to %s was turned down at chunk %d because of SERVER ERROR. Error message from server: %s", m.Protocol, peerID, chunkIndex, msg)
					return nil
				}, func() {
					log.Debugf("Responses of peer %s stopped after %d response chunks", peerID.Pretty(), lastRespChunkIndex+1)
					onClose(peerID)
				}); err != nil {
				log.Errorf("failed request: %v", err)
			}
		}
		return cmd
	}

	makeRespCmd := func(cmd *cobra.Command,
		rpcMethod func(cmd *cobra.Command) *reqresp.RPCMethod,
		handleReqFactory func(cmd *cobra.Command, args []string) (reqresp.ChunkedRequestHandler, error),
	) *cobra.Command {
		cmd.Flags().String("compression", "none", "Optional compression. Try 'snappy' for streaming-snappy")
		cmd.Flags().String("invalid-input", "", "If specified, an InvalidRequest(1) will be sent with the given message.")
		cmd.Flags().String("server-error", "", "If specified, an ServerError(2) will be sent with the given message.")
		cmd.Run = func(cmd *cobra.Command, args []string) {
			if cmd.Flags().Changed("invalid-input") && cmd.Flags().Changed("server-error") {
				log.Error("cannot write both invalid-input and server-error to response")
				return
			}
			if r.NoHost(log) {
				return
			}
			sCtxFn := func() context.Context {
				ctx, _ := context.WithTimeout(r.Ctx, time.Second*10) // TODO add timeout option
				return ctx
			}
			comp, err := readOptionalComp(cmd)
			if err != nil {
				log.Error(err)
				return
			}
			m := rpcMethod(cmd)
			handleReq, err := handleReqFactory(cmd, args)
			if err != nil {
				log.Error(err)
				return
			}
			handleReqWrap := func(ctx context.Context, peerId peer.ID, request reqresp.Request, respChunk reqresp.WriteSuccessChunkFn, respChunkInvalidInput reqresp.WriteMsgFn, respChunkServerError reqresp.WriteMsgFn) error {
				log.Debugf("Got request from %s, protocol %s: %s", peerId.Pretty(), m.Protocol, request.String())
				respChunkWrap := func(data interface{}) error {
					log.Debugf("Responding SUCCESS to peer %s with data: %v", peerId.Pretty(), data)
					return respChunk(data)
				}
				respInvalidInputWrap := func(msg string) error {
					log.Debugf("Responding INVALID INPUT to peer %s with message: '%s'", peerId.Pretty(), msg)
					return respChunkInvalidInput(msg)
				}
				respServerErrWrap := func(msg string) error {
					log.Debugf("Responding SERVER ERROR to peer %s with message: '%s'", peerId.Pretty(), msg)
					return respChunkInvalidInput(msg)
				}
				if cmd.Flags().Changed("invalid-input") {
					v, _ := cmd.Flags().GetString("invalid-input")
					return respChunkInvalidInput(v)
				}
				if cmd.Flags().Changed("server-error") {
					v, _ := cmd.Flags().GetString("server-error")
					return respChunkServerError(v)
				}
				return handleReq(ctx, peerId, request, respChunkWrap, respInvalidInputWrap, respServerErrWrap)
			}
			onInvalidInput := func(ctx context.Context, peerId peer.ID, err error) {
				log.Warnf("Got invalid input from %s, protocol %s, err: %v", peerId.Pretty(), m.Protocol, err)
			}
			onServerError := func(ctx context.Context, peerId peer.ID, err error) {
				log.Warnf("Server error on request from %s, protocol %s, err: %v", peerId.Pretty(), m.Protocol, err)
			}
			streamHandler, err := m.MakeStreamHandler(sCtxFn, comp, handleReqWrap, onInvalidInput, onServerError)
			if err != nil {
				log.Error(err)
				return
			}
			r.P2PHost.SetStreamHandler(m.Protocol, streamHandler)
		}
		return cmd
	}

	responseNotImplemented := func(cmd *cobra.Command, args []string) (handler reqresp.ChunkedRequestHandler, err error) {
		return func(ctx context.Context, peerId peer.ID, request reqresp.Request, respChunk reqresp.WriteSuccessChunkFn, onInvalidInput reqresp.WriteMsgFn, onServerErr reqresp.WriteMsgFn) error {
			log.Infof("Received request: %s", request.String())
			return fmt.Errorf("response-type is not implemented to make success responses. Ignoring request and closing stream")
		}, nil
	}

	goodbyeCmd := &cobra.Command{
		Use:   "goodbye",
		Short: "Manage goodbye RPC",
	}
	goodbyeCmd.AddCommand(makeReqCmd(&cobra.Command{
		Use:   "req <peerID> <code>",
		Short: "Send a goodbye to a peer, optionally disconnecting the peer after sending the Goodbye.",
		Args:  cobra.ExactArgs(2),
	}, func(cmd *cobra.Command) *reqresp.RPCMethod {
		return &methods.GoodbyeRPCv1
	}, func(cmd *cobra.Command, args []string) (reqresp.Request, error) {
		v, err := strconv.ParseUint(args[1], 0, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Goodbye code '%s'", args[1])
		}
		req := methods.Goodbye(v)
		return &req, nil
	}, func(peerID peer.ID, chunkIndex uint64, readChunk func(dest interface{}) error) error {
		if chunkIndex > 0 {
			return fmt.Errorf("unexpected second Goodbye response chunk from peer %s", peerID.Pretty())
		}
		var data methods.Goodbye
		if err := readChunk(&data); err != nil {
			return err
		}
		log.Infof("Goodbye RPC response of peer %s: %d", peerID.Pretty(), data)
		return nil
	}, func(peerID peer.ID) {
		log.Infof("Goodbye RPC responses of peer %s ended", peerID.Pretty())
	},
	))
	goodbyeCmd.AddCommand(makeRespCmd(&cobra.Command{
		Use:   "resp",
		Short: "Respond to peers.",
		Args:  cobra.NoArgs,
	}, func(cmd *cobra.Command) *reqresp.RPCMethod {
		return &methods.GoodbyeRPCv1
	}, responseNotImplemented))
	cmd.AddCommand(goodbyeCmd)

	parseRoot := func(v string) ([32]byte, error) {
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
	parseForkVersion := func(v string) ([4]byte, error) {
		if strings.HasPrefix(v, "0x") {
			v = v[2:]
		}
		if len(v) != 8 {
			return [4]byte{}, fmt.Errorf("provided root has length %d, expected 8 hex characters (ignoring optional 0x prefix)", len(v))
		}
		var out [4]byte
		_, err := hex.Decode(out[:], []byte(v))
		return out, err
	}

	blocksByRangeCmd := &cobra.Command{
		Use:   "blocks-by-range",
		Short: "Manage blocks-by-range RPC",
	}
	chooseBlocksByRangeVersion := func(cmd *cobra.Command) *reqresp.RPCMethod {
		v2, _ := cmd.Flags().GetBool("v2")
		if v2 {
			return &methods.BlocksByRangeRPCv2
		} else {
			return &methods.BlocksByRangeRPCv1
		}
	}
	blocksByRangeReqCmd := makeReqCmd(&cobra.Command{
		Use:   "req <peerID> <start-slot> <count> <step> [head-root-hex]",
		Short: "Get blocks by range from a peer. The head-root is optional, and defaults to zeroes. Use --v2 for no head-root.",
		Args:  cobra.RangeArgs(4, 5),
	}, chooseBlocksByRangeVersion, func(cmd *cobra.Command, args []string) (reqresp.Request, error) {
		startSlot, err := strconv.ParseUint(args[1], 0, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse start slot '%s'", args[1])
		}
		count, err := strconv.ParseUint(args[2], 0, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse count '%s'", args[1])
		}
		step, err := strconv.ParseUint(args[3], 0, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse step '%s'", args[1])
		}
		v2, err := cmd.Flags().GetBool("v2")
		if err != nil {
			return nil, err
		}
		if v2 {
			return &methods.BlocksByRangeReqV2{
				StartSlot: methods.Slot(startSlot),
				Count:     count,
				Step:      step,
			}, nil
		} else {
			var root [32]byte
			if len(args) > 4 {
				root, err = parseRoot(args[4])
				if err != nil {
					return nil, err
				}
			}
			return &methods.BlocksByRangeReqV1{
				HeadBlockRoot: root,
				StartSlot:     methods.Slot(startSlot),
				Count:         count,
				Step:          step,
			}, nil
		}
	}, func(peerID peer.ID, chunkIndex uint64, readChunk func(dest interface{}) error) error {
		var data methods.SignedBeaconBlock
		if err := readChunk(&data); err != nil {
			return err
		}
		log.Infof("Block RPC response of peer %s: Slot: %d Parent root: %x Sig: %x", peerID.Pretty(), data.Message.Slot, data.Message.ParentRoot, data.Signature)
		return nil
	}, func(peerID peer.ID) {
		log.Infof("Blocks-by-range RPC responses of peer %s ended", peerID.Pretty())
	},
	)
	blocksByRangeReqCmd.Flags().Bool("v2", false, "To use v2 (no head root in request)")
	blocksByRangeCmd.AddCommand(blocksByRangeReqCmd)

	blocksByRangeRespCmd := makeRespCmd(&cobra.Command{
		Use:   "resp",
		Short: "Respond to peers.",
		Args:  cobra.NoArgs,
	}, chooseBlocksByRangeVersion, responseNotImplemented)
	blocksByRangeRespCmd.Flags().Bool("v2", false, "To use v2 (no head root in request)")

	blocksByRangeCmd.AddCommand(blocksByRangeRespCmd)

	cmd.AddCommand(blocksByRangeCmd)

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Manage status RPC",
	}
	statusCmd.AddCommand(&cobra.Command{
		Use:   "view",
		Short: "Show current status",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			log.Infof("Current status: %s", state.CurrentStatus.String())
		},
	})
	parseStatus := func(args []string) (*methods.Status, error) {
		forkVersion, err := parseForkVersion(args[0])
		if err != nil {
			return nil, err
		}
		finalizedRoot, err := parseRoot(args[1])
		if err != nil {
			return nil, err
		}
		finalizedEpoch, err := strconv.ParseUint(args[2], 0, 64)
		if err != nil {
			return nil, err
		}
		headRoot, err := parseRoot(args[3])
		if err != nil {
			return nil, err
		}
		headSlot, err := strconv.ParseUint(args[4], 0, 64)
		if err != nil {
			return nil, err
		}
		return &methods.Status{
			HeadForkVersion: forkVersion,
			FinalizedRoot:   finalizedRoot,
			FinalizedEpoch:  methods.Epoch(finalizedEpoch),
			HeadRoot:        headRoot,
			HeadSlot:        methods.Slot(headSlot),
		}, nil
	}
	statusCmd.AddCommand(&cobra.Command{
		Use:   "set <head-fork-version> <finalized-root> <finalized-epoch> <head-root> <head-slot>",
		Short: "Change current status.",
		Args:  cobra.ExactArgs(5),
		Run: func(cmd *cobra.Command, args []string) {
			stat, err := parseStatus(args)
			if err != nil {
				log.Error(err)
				return
			}
			state.CurrentStatus = *stat
			log.Infof("Set to status: %s", state.CurrentStatus.String())
		},
	})
	statusCmd.AddCommand(makeReqCmd(&cobra.Command{
		Use:   "req <peerID> [<head-fork-version> <finalized-root> <finalized-epoch> <head-root> <head-slot>]",
		Short: "Ask peer for status. Request with given status, or current status if not defined.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 && len(args) != 6 {
				return fmt.Errorf("accepts either 1 or 6 args, received %d", len(args))
			}
			return nil
		},
	}, func(cmd *cobra.Command) *reqresp.RPCMethod {
		return &methods.StatusRPCv1
	}, func(cmd *cobra.Command, args []string) (reqresp.Request, error) {
		if len(args) != 1 {
			reqStatus, err := parseStatus(args[1:])
			if err != nil {
				return nil, err
			}
			return reqStatus, nil
		} else {
			return &state.CurrentStatus, nil
		}
	}, func(peerID peer.ID, chunkIndex uint64, readChunk func(dest interface{}) error) error {
		log.Infof("status resp %d", chunkIndex)
		if chunkIndex > 0 {
			return fmt.Errorf("unexpected second Status response chunk from peer %s", peerID.Pretty())
		}
		var data methods.Status
		if err := readChunk(&data); err != nil {
			return err
		}
		log.Infof("Status RPC response of peer %s: %s", peerID.Pretty(), data.String())
		return nil
	}, func(peerID peer.ID) {
		log.Infof("Status RPC responses of peer %s ended", peerID.Pretty())
	},
	))
	statusCmd.AddCommand(makeRespCmd(&cobra.Command{
		Use:   "resp",
		Short: "Respond to peers with current status.",
		Args:  cobra.NoArgs,
	}, func(cmd *cobra.Command) *reqresp.RPCMethod {
		return &methods.StatusRPCv1
	}, func(cmd *cobra.Command, args []string) (handler reqresp.ChunkedRequestHandler, err error) {
		return func(ctx context.Context, peerId peer.ID, request reqresp.Request, respChunk reqresp.WriteSuccessChunkFn, onInvalidInput reqresp.WriteMsgFn, onServerErr reqresp.WriteMsgFn) error {
			return respChunk(&state.CurrentStatus)
		}, nil
	}))

	cmd.AddCommand(statusCmd)
	return cmd
}


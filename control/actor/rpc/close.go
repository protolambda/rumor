package rpc

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
)

type RpcMethodCloseCmd struct {
	*base.Base
	*RpcMethodData
	ReqId string `ask:"<req-id>" help:"the ID of the request to close"`
}

func (c *RpcMethodCloseCmd) Help() string {
	return "Close open requests"
}

func (c *RpcMethodCloseCmd) Run(ctx context.Context, args ...string) error {
	key, req, err := c.checkAndGetReq(c.ReqId)
	if err != nil {
		return err
	}
	c.Responder.CloseRequest(key)
	c.Log.WithFields(logrus.Fields{
		"req_id": key,
		"peer":   req.From.String(),
	}).Infof("Closed request.")
	return nil
}

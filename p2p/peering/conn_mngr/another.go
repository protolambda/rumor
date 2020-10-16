package conn_mngr

import (
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"time"
)

type AutoMng struct {
	*connmgr.BasicConnMgr
}

func NewAutoMng(low, hi int, grace time.Duration, opts ...connmgr.Option) *AutoMng {
	return &AutoMng{connmgr.NewConnManager(low, hi, grace, opts...)}
}


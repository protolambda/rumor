package flags

import (
	"fmt"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
)

type CompressionFlag struct {
	Compression reqresp.Compression
}

func (f *CompressionFlag) String() string {
	if f == nil {
		return "nil compression"
	}
	if f.Compression == nil {
		return "none"
	}
	return f.Compression.Name()
}

func (f *CompressionFlag) Set(v string) error {
	if v == "snappy" {
		f.Compression = reqresp.SnappyCompression{}
		return nil
	} else if v == "none" || v == "" {
		f.Compression = nil
		return nil
	} else {
		return fmt.Errorf("unrecognized compression: %s", v)
	}
}

func (f *CompressionFlag) Type() string {
	return "RPC compression"
}

module github.com/protolambda/rumor

go 1.13

require (
	github.com/btcsuite/btcd v0.20.1-beta
	github.com/chzyer/logex v1.1.10 // indirect
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e
	github.com/chzyer/test v0.0.0-20180213035817-a1ea475d72b1 // indirect
	github.com/ethereum/go-ethereum v1.9.13
	github.com/golang/snappy v0.0.1
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/gorilla/websocket v1.4.2
	github.com/ipfs/go-datastore v0.4.4
	github.com/libp2p/go-libp2p v0.8.1
	github.com/libp2p/go-libp2p-connmgr v0.2.1
	github.com/libp2p/go-libp2p-core v0.5.1
	github.com/libp2p/go-libp2p-mplex v0.2.3
	github.com/libp2p/go-libp2p-noise v0.1.1
	github.com/libp2p/go-libp2p-peerstore v0.2.2
	github.com/libp2p/go-libp2p-pubsub v0.2.6
	github.com/libp2p/go-libp2p-secio v0.2.2
	github.com/libp2p/go-libp2p-tls v0.1.3
	github.com/libp2p/go-libp2p-yamux v0.2.7
	github.com/libp2p/go-tcp-transport v0.2.0
	github.com/libp2p/go-ws-transport v0.3.0
	github.com/minio/sha256-simd v0.1.1
	github.com/multiformats/go-base32 v0.0.3
	github.com/multiformats/go-multiaddr v0.2.1
	github.com/protolambda/ask v0.0.1
	github.com/protolambda/zrnt v0.12.2-alpha.0
	github.com/protolambda/zssz v0.1.5
	github.com/protolambda/ztyp v0.0.1
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
)

replace (
	github.com/protolambda/ask => ../ask
	github.com/protolambda/zrnt => ../zrnt
)

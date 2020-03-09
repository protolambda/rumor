# Rumor

A REPL written in Go, to run the Eth2 network stack, attach to testnets, debug clients, and extract data for tooling.

## Install

This REPL relies on a forked go-ethereum for discovery-v5, this requires you to run it from source (instead of `go get` to install).

```shell script
git clone git@github.com:protolambda/rumor.git
cd rumor
go get ./...
```

## Usage

To run it in interactive mode, a.k.a. REPL functionality:

```shell script
go run . -i
```

Alternatively, provide the commands by piping into std-in, or use a single argument to give a file-path to read from. 

## General

```shell script
# Have a look at all functionality
help

# Start the libp2p host
host start

# Start listening (optionally specify ip or tcp/udp ports to listen at)
# Note that the peer-manager is active by default.
host listen

# See the host up and running
host view

# Broswe your peers
peer list

# Connect to peer
peer connect <enr/enode/multi-addr>
```

### Connecting to testnets

Prysm:
```shell script
# Start kademlia and connect to the Prysm testnet
kad start /prysm/0.0.0/dht
# Connect to bootnode
peer connect /dns4/prylabs.net/tcp/30001/p2p/16Uiu2HAm7Qwe19vz9WzD2Mxn7fXd1vgHHp4iccuyq7TxwRXoAGfc
# Protect bootnode
peer protect 16Uiu2HAm7Qwe19vz9WzD2Mxn7fXd1vgHHp4iccuyq7TxwRXoAGfc bootnode
# Refresh the kademlia table
kad refresh
```

Lighthouse:
```shell script
# Start discv5 and connect to the Lighthouse testnet
# Give one or more bootnode addresses (ENR or enode format).
dv5 start enr:-Iu4QLNTiVhgyDyvCBnewNcn9Wb7fjPoKYD2NPe-jDZ3_TqaGFK8CcWr7ai7w9X8Im_ZjQYyeoBP_luLLBB4wy39gQ4JgmlkgnY0gmlwhCOhiGqJc2VjcDI1NmsxoQMrmBYg_yR_ZKZKoLiChvlpNqdwXwodXmgw_TRow7RVwYN0Y3CCIyiDdWRwgiMo
# Get your local Discv5 node info. Other discv5 nodes can bootstrap from this.
dv5 self
# Get nearby nodes
dv5 lookup
```

```shell script
# Log gossip blocks of prysmatic testnet to file
gossip start
gossip join /eth2/beacon_block/ssz
# listening on the topic!
gossip list
# Update your kademlia peers to learn about the gossip topic interest.
kad refresh
# Check your peers on the topic
gossip list-peers /eth2/beacon_block/ssz
# Start logging
gossip log start beacon_block /eth2/beacon_block/ssz blocks.txt
# Show gossip loggers
gossip log list
# Stop block logger
gossip log stop beacon_block
# Leave channel
gossip leave /eth2/beacon_block/ssz

# Ask a connected node for a Status
rpc status req 16Uiu2HAmQ9WByeSsnxLb2tBW3MkGYMfg1BQowkwyVdUD9WwMdnrc
```

### Actors

By prefixing a command with `<actor-name>:`, you can run multiple p2p hosts in the same REPL process.

```
alice: host start
alice: host listen --tcp=9000
bob: host start
bob: host listen --tcp=9001
# Get ENR of alice
alice: host view
# Connect bob to alice
bob: peer connect <ENR from alice>
```

## License

MIT, see [`LICENSE`](./LICENSE) file.

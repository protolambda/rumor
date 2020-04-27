# Rumor

A REPL written in Go, to run the Eth2 network stack, attach to testnets, debug clients, and extract data for tooling.

## Install

To just install it in one command:

```shell script
GO111MODULE=on go get github.com/protolambda/rumor
```

To run it from source (instead of `go get` to install)

```shell script
git clone git@github.com:protolambda/rumor.git
cd rumor
go get ./...
```

## Usage

There are a few subcommands to choose the mode of operation:

```
  shell       Rumor as a human-readable shell
  bare        Rumor as a bare JSON-formatted input/output process, suitable for use as subprocess. Optionally read input from a file instead of stdin.
 
  serve       Rumor as a server to attach to
  attach      Attach to a running rumor server
```

## General

```shell script
# Have a look at all functionality
help

# Start the libp2p host
host start

# You probably want to know everything that is happening, run this in the background:
bg host notify all

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
# Subscribe to the topic
gossip log /eth2/beacon_block/ssz
# Exit task
cancel
# Leave channel
gossip leave /eth2/beacon_block/ssz

# Ask a connected node for a Status
rpc status req 16Uiu2HAmQ9WByeSsnxLb2tBW3MkGYMfg1BQowkwyVdUD9WwMdnrc
```

And there are commands to make debugging various things easy as well:
```
enr view --kv enr:-Iu4QLNTiVhgyDyvCBnewNcn9Wb7fjPoKYD2NPe-jDZ3_TqaGFK8CcWr7ai7w9X8Im_ZjQYyeoBP_luLLBB4wy39gQ4JgmlkgnY0gmlwhCOhiGqJc2VjcDI1NmsxoQMrmBYg_yR_ZKZKoLiChvlpNqdwXwodXmgw_TRow7RVwYN0Y3CCIyiDdWRwgiMo
```

### Actors

By prefixing a command with `{actor-name}:`, you can run multiple p2p hosts, by name `{actor-name}` in the same REPL process.

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

### Call IDs

By prefixing a command with `{my-call-id}>`, you can see which logs are a result of the given call `{my-call-id}`,
 even if interleaved with other logs (because of async or concurrent results).
The call-ID should be placed before the actor name.
If no call-ID is specified, the call uses an incrementing counter for each new call. I.e. an auto-generated call ID, with just a number, the command is prefixed with e.g. `123> `.
Using bare numbers as custom call-IDs is unsafe, as these call-IDs will overlap with the auto-generated IDs.

```
my_call> alice: host start
.... log output with call_id="my_call"
```

The call-ID is useful to correlate results, and for tooling that interacts with Rumor to get results. 

### Background / foreground

Calls can be made in, and moved to, foreground and background.

#### Making calls
 
- After a command is started, it is in the foreground by default, i.e. the shell waits for it to run the next command.
- To start a command in the background instead, add `bg` before the actual command. E.g. `example_host_call> alice: bg host start`

#### Moving calls

- To put the current foreground call in the background, enter `bg`.
- To put a specific call back in the foreground, enter `my_call_id> fg`.

### Cancel calls

To close a long-running call (which may run in the background), you can:
- Move it to foreground if it is not already, then cancel it:
```
123> fg
cancel
```
- Cancel it directly:
```
123> cancel
```

### Exit

Enter `exit`, or send an interrupt signal.
All remaining open jobs will be canceled.

## License

MIT, see [`LICENSE`](./LICENSE) file.

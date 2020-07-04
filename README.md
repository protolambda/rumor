# Rumor

An interactive shell written in Go, to run the Eth2 network stack, attach to testnets, debug clients, and extract data for tooling.

The shell is built on top of [`mvdan/sh`](https://github.com/mvdan/sh), which aims to be POSIX compatible, and has some Bash features.

The core idea of the shell is to start and maintain p2p processes, and make the resulting information available though logs, 
 to auto-fill the environment with anything you would want.

Also note that actors and running commands can be shared and persisted between sessions.

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

## Built with

- Eth2:
    - [`protolambda/zrnt`](https://github.com/protolambda/zrnt): For Eth2 consensus code
    - [`protolambda/ztyp`](https://github.com/protolambda/ztyp): For SSZ tree structures
    - [`protolambda/ztyp`](https://github.com/protolambda/ztyp): For SSZ utils (incl streaming for RPC)
    - [`ethereum/go-ethereum`](https://github.com/ethereum/go-ethereum) Geth, for ENR and Discv5 packages
- Libp2p
    - [`ipfs/go-datastore`](https://github.com/ipfs/go-datastore): New experimental peerstore code
    - [`libp2p/go-libp2p`](https://github.com/libp2p/go-libp2p): Gossipsub, mutli-select, transports, multiplexers, identification, RPC base, and more.
- [`mvdan/sh`](https://github.com/mvdan/sh): For shell parsing and interpreting
- [`chzyer/readline`](https://github.com/chzyer/readline): For history/shell code
- [`gorilla/websocket`](https://github.com/gorilla/websocket): For websocket server/client
- [`sirupsen/logrus`](https://github.com/sirupsen/logrus): For logging infrastructure

## Usage

There are a few subcommands to choose the mode of operation:

```
  attach      Attach to a running rumor server
  bare        Rumor as a bare JSON-formatted input/output process, suitable for use as subprocess.
  file        Run rom a file
  help        Help about any command
  serve       Rumor as a server to attach to
  shell       Rumor as a human-readable shell
```

Serve and attach supported types: `ws` (http upgraded to websocket, optional auth key), `tcp` (raw socket), `ipc` (Unix socket)

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
dv5 run enr:-Iu4QLNTiVhgyDyvCBnewNcn9Wb7fjPoKYD2NPe-jDZ3_TqaGFK8CcWr7ai7w9X8Im_ZjQYyeoBP_luLLBB4wy39gQ4JgmlkgnY0gmlwhCOhiGqJc2VjcDI1NmsxoQMrmBYg_yR_ZKZKoLiChvlpNqdwXwodXmgw_TRow7RVwYN0Y3CCIyiDdWRwgiMo
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

By prefixing a command with `{actor-name}:`, you can run multiple p2p hosts, by name `{actor-name}` in the same process.

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

To change the default actor, use the `me` command:

```
protolambda: me
```

The environment var `ACTOR` is reserved and always points to the current default actor name
 (not to what the command may say, which is evaluated later).

### Includes

Writing some things may become repetitive.
To not repeat yourself, while still getting the same results as typing it yourself, you can use `include` to run rumor files.

`host.rumor`
```
host start
host notify all
# maybe start some other tasks
```

Then, in a shell, or in another rumor file:
```
# The include will use the new default actor
alice: me
include host.rumor
alice: host view
```

### Log levels

The loglevel individual commands can be changed with the `lvl_{name}` proxy commands.
`name` can be any of `trace, debug, info, warn, error, fatal, panic`
E.g. `lvl_warn peer connect` to ignore most logs, except warnings and higher-severity errors.


### Call IDs

By prefixing a command with `_`, like `_{my-call-id}`, you can see which logs are a result of the given call `{my-call-id}`,
 even if interleaved with other logs (because of async results).
The call-ID should be placed before the actor name.
If no call-ID is specified, the call uses an incrementing counter for each new call.
I.e. an auto-generated call ID, with just the session id and a number, the command is prefixed with e.g. `s42_123`.
Using such bare strings as custom call-IDs is unsafe, as these call-IDs will overlap with the auto-generated IDs.

```
_my_call alice: host start
.... log output with call_id="my_call"

# also valid, but not recommend:
alice: _my_call host start
```

The call-ID is useful to correlate results, and for tooling that interacts with Rumor to get results.

Also, to reference log-output in environment variables, the call IDs are very useful to select an exact variable.

### Automatic environment variables

Each log-entry will automatically be stored in the Rumor environment, to retrieve by its call ID.

Example:
```
_my_call alice: host listen
bob: peer connect _my_call_enr
```

And, the log entry variables of the last call in the session can be referenced with just `_` (and still separated with `_` from the log-entry name):

Example:
```
alice: host listen
bob: peer connect __enr
```

Both the call ID and the log entry name may contain multiple underscores. References to call IDs always start with a `_`.

After running many calls, the environment may grow too big.
To flush it, run `clear_log_data` to clear every entry, except those prefixed with a call ID of an open call.


### Cancel long-running calls

To close a long-running call (which may run in the background), you can:
- Move it to foreground if it is not already, then cancel it:
```
cancel
```
- Cancel it directly by its call ID:
```
_my_call_ cancel
```

### Stopping an actor

An actor can be killed by running a `kill` command on them, after that a fresh libp2p host can be started.

```
alice: kill
```

### Exit

Enter `exit`, or send an interrupt signal.
All remaining open calls will be canceled.

### Listing calls

To see what Rumor is running, use `calls`

### Shell built-ins

These are reserved names, used for shell functionality:

```
"false", "exit", "set", "shift", "unset",
"echo", "printf", "break", "continue", "pwd", "cd",
"wait", "builtin", "trap", "type", "source", ".", "command",
"dirs", "pushd", "popd", "umask", "alias", "unalias",
"fg", "bg", "getopts", "eval", "test", "[", "exec",
"return", "read", "shopt"
```

## License

MIT, see [`LICENSE`](./LICENSE) file.

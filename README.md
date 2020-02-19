# Rumor

A REPL written in Go, to run the Eth2 network stack, attach to testnets, debug clients, and extract data for tooling.

## Usage

```bash
# Have a look at all functionality
help

# Start the libp2p host
host start

# Start listening (optionally specify multi addrs to listen at)
host listen

# See the host up and running
host view

# Start kademlia and connect to the Prysm testnet
kad start /prysm/0.0.0/dht
# Connect to bootnode
peer connect /dns4/prylabs.net/tcp/30001/p2p/16Uiu2HAm7Qwe19vz9WzD2Mxn7fXd1vgHHp4iccuyq7TxwRXoAGfc
# Protect bootnode
peer protect 16Uiu2HAm7Qwe19vz9WzD2Mxn7fXd1vgHHp4iccuyq7TxwRXoAGfc bootnode
# Refresh the kademlia table
kad refresh

# Start discv5 and connect to the Lighthouse testnet
# Give one or more bootnode addresses (ENR or enode format).
dv5 start enr:-Iu4QLNTiVhgyDyvCBnewNcn9Wb7fjPoKYD2NPe-jDZ3_TqaGFK8CcWr7ai7w9X8Im_ZjQYyeoBP_luLLBB4wy39gQ4JgmlkgnY0gmlwhCOhiGqJc2VjcDI1NmsxoQMrmBYg_yR_ZKZKoLiChvlpNqdwXwodXmgw_TRow7RVwYN0Y3CCIyiDdWRwgiMo
# Get your local Discv5 node info. Other discv5 nodes can bootstrap from this.
dv5 self
# Get nearby nodes
dv5 lookup

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
```

## License

MIT, see [`LICENSE`](./LICENSE) file.

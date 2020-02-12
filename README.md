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
dv5 start 0.0.0.0:9000
# Get your local Discv5 node info
dv5 self
# Bootstrap discv5 with a bootnode ENR (see help for other addr formats)
# Alternatively, use the above info from another REPL instance to make an enode address, and bootstrap to that.
dv5 bootstrap enr:-Iu4QLNTiVhgyDyvCBnewNcn9Wb7fjPoKYD2NPe-jDZ3_TqaGFK8CcWr7ai7w9X8Im_ZjQYyeoBP_luLLBB4wy39gQ4JgmlkgnY0gmlwhCOhiGqJc2VjcDI1NmsxoQMrmBYg_yR_ZKZKoLiChvlpNqdwXwodXmgw_TRow7RVwYN0Y3CCIyiDdWRwgiMo
# Get nearby nodes
dv5 nearby

```
## License

MIT, see [`LICENSE`](./LICENSE) file.

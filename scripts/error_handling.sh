LISTEN_IP=127.0.0.1
LISTEN_PORT=13000

host start
host listen --ip=$LISTEN_IP --tcp=$LISTEN_PORT

# Connect to non-existant node, check if error is handled as expected
REMOTE_PEER_ID=16Uiu2HAmP5zsn2Uw7vJyLmXLF7yh74pSDTeq15wtQMVZfs37yh1p
REMOTE_IP=192.168.99.99
REMOTE_PORT=9000
echo pre
peer connect /ip4/$REMOTE_IP/tcp/$REMOTE_PORT/p2p/$REMOTE_PEER_ID || true
echo post

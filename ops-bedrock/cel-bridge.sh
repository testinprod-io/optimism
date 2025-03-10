#!/bin/sh

# Wait for Celestia App to be ready
while ! curl -s http://celestia-app:26657/status > /dev/null; do
  echo 'Waiting for Celestia App...'
  sleep 2
done

# Fetch Genesis block hash
GENESIS=
CNT=0
MAX=30
while [ -z "$GENESIS" ] || [ "$GENESIS" == "null" ] && [ $CNT -lt $MAX ]; do
    GENESIS=$(curl -s http://celestia-app:26657/block?height=1 | jq -r '.result.block_id.hash' | tr -d '"')
    echo "GENESIS: $GENESIS"
    CNT=$((CNT + 1))
    sleep 1
done

if [ -z "$GENESIS" ] || [ "$GENESIS" == "null" ]; then
    echo "Error: Could not retrieve Genesis block hash. Exiting..."
    exit 1
fi

export CELESTIA_CUSTOM=test:$GENESIS
echo "Genesis hash: $CELESTIA_CUSTOM"

# Initialize and start bridge
celestia bridge init \
  --core.ip host.docker.internal \
  --p2p.network test \
  --rpc.skip-auth \
  --rpc.addr 0.0.0.0 \
  --keyring.backend test \
  --keyring.keyname my_celes_key \
  --rpc.port 26658

celestia bridge start \
  --core.ip host.docker.internal \
  --p2p.network test \
  --rpc.skip-auth \
  --rpc.addr 0.0.0.0 \
  --rpc.port 26658 \
  --keyring.backend test \
  --keyring.keyname my_celes_key &

echo "Bridge is ready! Creating shared directory and file..."
mkdir -p /shared
touch /shared/bridge_ready

# Keep container running
tail -f /dev/null

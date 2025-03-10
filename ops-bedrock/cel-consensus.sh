#!/bin/sh

celestia-appd tendermint unsafe-reset-all --home $HOME/.celestia-app

VALIDATOR_NAME=validator1
CHAIN_ID=test
KEY_NAME=validator
TIA_AMOUNT="1000000000000000000utia"
STAKING_AMOUNT="9000000utia"
FEE_AMOUNT="500utia"

celestia-appd init $VALIDATOR_NAME --chain-id $CHAIN_ID
celestia-appd keys add $KEY_NAME --keyring-backend test
celestia-appd add-genesis-account $KEY_NAME $TIA_AMOUNT --keyring-backend test
celestia-appd gentx $KEY_NAME $STAKING_AMOUNT --keyring-backend test --chain-id $CHAIN_ID --fees $FEE_AMOUNT
celestia-appd collect-gentxs

sed -i 's/^laddr = "tcp:\/\/127.0.0.1:26657"/laddr = "tcp:\/\/0.0.0.0:26657"/' $HOME/.celestia-app/config/config.toml
sed -i 's/^timeout_commit.*$/timeout_commit = "2s"/' $HOME/.celestia-app/config/config.toml
sed -i 's/^timeout_propose.*$/timeout_propose = "2s"/' $HOME/.celestia-app/config/config.toml

# Start celestia-appd in the background
celestia-appd start --force-no-bbr --grpc.enable true --rpc.laddr tcp://0.0.0.0:26657 &

echo "Waiting for celestia-bridge to be ready..."
while [ ! -f /shared/bridge_ready ]; do
    echo "Checking for /shared/bridge_ready file..."
    ls -la /shared || true
    sleep 5
done

sleep 10

echo "Bridge is ready! Now fund the celestia bridge address..."

FROM_ADDRESS=$(celestia-appd keys show validator -a --keyring-backend test)
echo "From address: $FROM_ADDRESS"

AMOUNT="1000000000000000utia"

echo "Fetching node address from bridge..."
NODE_ADDRESS=$(curl -s -X POST -H "Content-Type: application/json" \
  --data '{"id":1,"jsonrpc":"2.0","method":"state.AccountAddress","params":[]}' \
  http://host.docker.internal:26658 | jq -r '.result')
echo "Node address: $NODE_ADDRESS"

if [ -z "$NODE_ADDRESS" ] || [ "$NODE_ADDRESS" = "null" ]; then
    echo "ERROR: Failed to get node address from bridge"
fi

echo "Sending transaction..."
celestia-appd tx bank send $FROM_ADDRESS $NODE_ADDRESS $AMOUNT --gas-prices 1utia --keyring-backend test --chain-id $CHAIN_ID -y
echo "Transaction sent, waiting 3 seconds..."
sleep 3
echo "Checking balance..."
celestia-appd q bank balances $NODE_ADDRESS

echo "All commands executed. Keeping container running..."
# Keep container running
tail -f /dev/null

#!/bin/sh
USING_OVM=true /home/ubuntu/optimism/l2geth/build/bin/geth --datadir=/home/ubuntu/goerli-legacy-archive --nousb --nodiscover --rpc --rpcapi "eth,net,web3,debug" console

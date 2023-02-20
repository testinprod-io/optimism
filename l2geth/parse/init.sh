#!/bin/sh
git clone -b pcw109550/l2geth@0.5.31 --single-branch https://github.com/testinprod-io/optimism.git && cd optimism/l2geth && make geth
cd /home/ubuntu/optimism/l2geth/parse && python3.10 -m venv venv && source venv/bin/activate && pip install -r requirements.txt
cd /home/ubuntu
wget https://storage.googleapis.com/oplabs-goerli-data/goerli-legacy-archival.tar
tar -xvf goerli-legacy-archival.tar
mkdir goerli-legacy-archive
mv geth goerli-legacy-archive/
mkdir -p goerli-legacy-archive/keystore
cd /home/ubuntu

# USING_OVM=true /home/ubuntu/optimism/l2geth/build/bin/geth --datadir=/home/ubuntu/goerli-legacy-archive --nousb --nodiscover --rpc --rpcapi "eth,net,web3,debug" console
# python3.10 /home/ubuntu/optimism/l2geth/parse/bench.py


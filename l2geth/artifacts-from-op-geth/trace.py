import web3
from web3.middleware import geth_poa_middleware
import pickle
import requests
from tqdm import tqdm
import sys
import time

start = int(sys.argv[1])
RPC = "http://localhost:8545"
w3 = web3.Web3(web3.HTTPProvider(RPC))
w3.middleware_onion.inject(geth_poa_middleware, layer=0)

addressSet = set()


body = {
    "jsonrpc": "2.0",
    "method": "debug_traceTransaction",
    "params": [
        "0xf55fea65f79adf55b4807a4418719d1f6a7e58264bd004ac75c35fa4965f3c25",
        {"tracer": "callTracer"}
    ],
    "id": 1
}


def trace(txHash):
    body["params"][0] = txHash
    while True:
        try:
            res = requests.post(RPC, json=body)
            break
        except:
            time.sleep(0.5)
            continue
    assert res.status_code == 200
    data = res.text.split('"from":')[1:]
    for d in data:    
        address_from = d[1: 1 + 42].lower()
        address_to   = d[2 + 42 + 7: 2 + 42 + 7 + 42].lower()
        addressSet.add(address_from)
        addressSet.add(address_to)


# 4061223

# 1 to 1000 * 1000 == 1000000

# 1 to 249
# 250 to 499
# 
print("iterating from ", 1 + 1000 * start, "to", 1000 * (start + 250))
fn = "trace_addr_" + str(1 + 1000 * start) + "_" + str(1000 * (start + 250))
print(fn)

for j in range(start, start + 250):
    print(1 + 1000 * j, 1000 * (j + 1) + 1)
    tx_list = []
    for i in range(1 + 1000 * j, 1000 * (j + 1) + 1):
        txs = w3.eth.getBlock(i)["transactions"]
        # prebedrock blocks only have single tx, ignoring genesis
        assert len(txs) == 1
        tx = txs[0].hex()
        tx_list.append(tx)

    for tx in tqdm(tx_list):
        trace(tx)

    print(len(addressSet))


with open(fn, "wb") as f:
    pickle.dump(addressSet, f)


# trace("0xf55fea65f79adf55b4807a4418719d1f6a7e58264bd004ac75c35fa4965f3c25")


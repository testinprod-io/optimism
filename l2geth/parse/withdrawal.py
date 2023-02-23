import json
import pickle

import eth_abi
import web3
from web3.middleware import geth_poa_middleware

RPC = "https://goerli.optimism.io"
w3 = web3.Web3(web3.HTTPProvider(RPC))
w3.middleware_onion.inject(geth_poa_middleware, layer=0)

topic = web3.Web3.keccak(
    text="SentMessage(address,address,bytes,uint256,uint256)"
).hex()

signature = web3.Web3.keccak(text="relayMessage(address,address,bytes,uint256)")[:4]

target_topics = [topic]

events = []

CHUNK = 100000
MAX_BLOCK = 4061223
fromBlock, toBlock = 0, CHUNK
for i in range(41):
    print(fromBlock, toBlock, fromBlock, min(MAX_BLOCK, toBlock))
    logs = w3.eth.get_logs(
        {
            "fromBlock": fromBlock,
            "toBlock": min(MAX_BLOCK, toBlock),
            "topics": [target_topics],
            "address": "0x4200000000000000000000000000000000000007",
        }
    )
    print(f"total log cnt : {len(logs)}")
    for log in logs:
        data = log["data"]
        topics = log["topics"]
        target = topics[1][-20:]
        decoded_data = eth_abi.decode(
            ["address", "bytes", "uint256", "uint256"], bytes.fromhex(data[2:])
        )
        sender, message, nonce = decoded_data[:-1]
        encoded_msg = (
            "0x"
            + (
                signature
                + eth_abi.encode(
                    ["address", "address", "bytes", "uint256"],
                    [target, bytes.fromhex(sender[2:]), message, nonce],
                )
            ).hex()
        )
        events.append(
            {"who": "0x4200000000000000000000000000000000000007", "msg": encoded_msg}
        )

    print(f"events len {len(events)}")

    fromBlock += CHUNK
    toBlock += CHUNK

# print(addressSet)
with open("legacy_withdraw_2.json", "w") as f:
    json.dump(events, f, indent=2)

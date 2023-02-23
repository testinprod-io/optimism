import json

import eth_abi
import rlp
import web3
from mpt import MerklePatriciaTrie


def update(trie, key, value):
    trie.update(bytes.fromhex(key), rlp.encode(bytes.fromhex(value)))


storage = {}
trie = MerklePatriciaTrie(storage, secure=True)  # storage trie is a secure trie

# add inital condition from l2geth last block 4061223

predeploy_storage = dict()
# from tei
with open("goerli-genesis-l2.json", "r") as f:
    goerli_genesis_l2_alloc = json.loads(f.read())["alloc"]
    for address, value in goerli_genesis_l2_alloc.items():
        address = "0x" + address
        if "storage" in value:
            predeploy_storage[address] = value["storage"]


final = dict()
for key, value in predeploy_storage[
    "0x4200000000000000000000000000000000000016"
].items():
    if int(value, 16) == 0:  # do not consider zero value
        continue
    key = key[2:] if key[:2] == "0x" else key
    if value[:2] == "0x":
        # value must have no 0x prefix and no leading zeros
        value = value[2:].lstrip("0")
        if len(value) % 2 == 1:
            value = "0" + value
    update(trie, key, value)
    final[key] = value

with open(
    "/Users/changwan.park/Documents/optimism/op-chain-ops/withdrawal_processed_output_2",
    "r",
) as f:
    for line in f.readlines():
        _, key, value = line.strip().split()
        key = key[2:] if key[:2] == "0x" else key
        value = value[2:] if value[:2] == "0x" else value
        value = value.lstrip("0")
        if len(value) % 2 == 1:
            value = "0" + value
        update(trie, key, value)
        final[key] = value

print("0x" + trie.root_hash().hex())
# target
print("0x3ed105f39087db62a27c353e0f130d6c2df93e69aad54e11daffc4dbb9c34e9b")

print(len(final.keys()))

# with open("legacy_withdraw.json", "r") as f:
#     data = json.loads(f.read())

# topic = web3.Web3.keccak(text="SentMessage(address,address,bytes,uint256,uint256)").hex()
# signature = web3.Web3.keccak(text='relayMessage(address,address,bytes,uint256)')[:4]

# for event in data:
#     who = event["who"]
#     encoded_msg = bytes.fromhex(event["msg"][2:])[len(signature):]
#     msg = eth_abi.decode(['address', 'address', 'bytes', 'uint256'], encoded_msg)
#     [target, sender, data, nonce] = msg
#     print(target, sender, data.hex(), nonce)

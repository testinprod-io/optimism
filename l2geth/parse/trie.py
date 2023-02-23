import rlp
from mpt import MerklePatriciaTrie

storage = {}
trie = MerklePatriciaTrie(storage, secure=True)  # storage trie is a secure trie
# print(f'empty root: {trie.root_hash().hex()}')

# key: zero-padded 32 bytes hex string
# value: hex string
def update(key, value):
    trie.update(bytes.fromhex(key), rlp.encode(bytes.fromhex(value)))


storage = {
    "0x0000000000000000000000000000000000000000000000000000000000000001": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc": "0x000000000000000000000000c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30016",
    "0xb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d6103": "0x0000000000000000000000004200000000000000000000000000000000000018",
}

for key, value in storage.items():
    key = key[2:] if key[:2] == "0x" else key
    if value[:2] == "0x":
        # value must have no 0x prefix and no leading zeros
        value = value[2:].lstrip("0")
        if len(value) % 2 == 1:
            value = "0" + value
    update(key, value)
print(trie.root_hash().hex())

# update("b53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d6103", "4200000000000000000000000000000000000018")
# print(trie.root_hash().hex())
# update("360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc", "c0D3C0d3C0d3C0D3c0d3C0d3c0D3C0d3c0d30000")


# how to make e188632b11db6a5c79d8f5a550184bdfba940587341f7180e771625707b67a04
# wrong!

# bedrock data wrong
# ./build/bin/geth --datadir=/Users/changwan.park/Downloads/goerli-bedrock-archive --nodiscover dump --iterative --incompletes --nocode 4061223 | grep 0x00b595b254a32979cf7cb1df529967bc2828d6f4bfb74190d178ccbe6ab1942d
# {"balance":"0","nonce":1,"root":"0xe188632b11db6a5c79d8f5a550184bdfba940587341f7180e771625707b67a04","codeHash":"0xa1ae4ae2461ac26aed16cb4252d3aba5c57b5d2d9738751ef3b67ef0db0ea58d",
# "storage":{"0x0000000000000000000000000000000000000000000000000000000000000000":
# "2121245dcad697f11244068aad6ecbc301811239"},"key":"0x00b595b254a32979cf7cb1df529967bc2828d6f4bfb74190d178ccbe6ab1942d"}
# legacy data correct
# USING_OVM=true ./build/bin/geth --datadir=/Users/changwan.park/Downloads/goerli-legacy-archive --nodiscover dump --iterative --nocode 4061223 | grep 0x62034e1cbb14e23809bf77c8fb105cfb3fdac76e
# "balance": "0",
# "nonce": 1,
# "root": "e188632b11db6a5c79d8f5a550184bdfba940587341f7180e771625707b67a04",
# "codeHash": "a1ae4ae2461ac26aed16cb4252d3aba5c57b5d2d9738751ef3b67ef0db0ea58d",
# "storage": {
#   "0x0000000000000000000000000000000000000000000000000000000000000000": "15",
#   "0x0000000000000000000000000000000000000000000000000000000000000001": "2121245dcad697f11244068aad6ecbc301811239"
# },
# "address": "0x62034e1cbb14e23809bf77c8fb105cfb3fdac76e"


# 0x2527617e608e162462d4098c8338838e3b87b562cd2096c05c7078777993827b

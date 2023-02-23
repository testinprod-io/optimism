import json
import pickle
from typing import Dict

import rlp
import web3
from mpt import MerklePatriciaTrie
from tqdm import tqdm

with open("final_preimage.pickle", "rb") as f:
    final_preimage = pickle.load(f)
print(len(final_preimage))

VALIDATE = True
"""
type ImportAccount struct {
	Balance   string                 `json:"balance"`
	Nonce     uint64                 `json:"nonce"`
	Root      string                 `json:"root"`
	CodeHash  string                 `json:"codeHash"`
	Code      string                 `json:"code,omitempty"`
	Storage   map[common.Hash]string `json:"storage,omitempty"`
}
"""


def encode_account(account):
    array = [
        int(account["nonce"]),
        int(account["balance"]),
        bytes.fromhex(account["root"]),
        bytes.fromhex(account["codeHash"]),
    ]
    return rlp.encode(array)


def create_account(balance, nonce, root, code, codeHash, storage) -> Dict:
    account = dict()
    account["balance"] = balance
    account["nonce"] = nonce
    # root must have no 0x prefix
    account["root"] = root[2:] if "0x" == root[:2] else root
    # codeHash must have no 0x prefix
    account["codeHash"] = codeHash[2:] if "0x" == codeHash[:2] else codeHash
    if code is not None:
        # code must have no 0x prefix
        account["code"] = code[2:] if "0x" == code[:2] else code
    if storage is not None:
        # common.Hash -> string
        new_storage = dict()
        for key, value in storage.items():
            # key must be 0x prefixed
            assert key[:2] == "0x"
            if value[:2] == "0x":
                # value must have no 0x prefix and no leading zeros
                value = value[2:].lstrip("0")
                if len(value) % 2 == 1:
                    value = "0" + value
            new_storage[key] = value
        account["storage"] = new_storage
    return account


def update(trie, key, value):
    trie.update(bytes.fromhex(key), rlp.encode(bytes.fromhex(value)))


def validate_trie(storage, root):
    trie_storage = {}
    trie = MerklePatriciaTrie(trie_storage, secure=True)
    for key, value in tqdm(storage.items()):
        key = key[2:] if key[:2] == "0x" else key
        if value[:2] == "0x":
            # value must have no 0x prefix and no leading zeros
            value = value[2:].lstrip("0")
            if len(value) % 2 == 1:
                value = "0" + value
        update(trie, key, value)

    new_root = "0x" + trie.root_hash().hex()
    assert new_root == root, (new_root, root)


def validate_code(code, codeHash):
    new_codeHash = web3.Web3.keccak(hexstr=code).hex()
    assert new_codeHash == codeHash, (new_codeHash, codeHash)


def hack_for_0x4200000000000000000000000000000000000016():
    storage = predeploy_storage["0x4200000000000000000000000000000000000016"]
    with open(
        "/Users/changwan.park/Documents/optimism/op-chain-ops/withdrawal_processed_output_2",
        "r",
    ) as f:
        for line in f.readlines():
            _, key, value = line.strip().split()
            storage[key] = value
    return storage


# state root: 0xbfe2b059bc76c33556870c292048f1d28c9d498462a02a3c7aadb6edf1c2d21c
# 600M    everything_4061224
result = dict()
# wrong storage content from bedrock, must use legacy

correct_storage = dict()
# USING_OVM=true ./build/bin/geth --datadir=/Users/changwan.park/Downloads/goerli-legacy-archive --nodiscover dump --iterative 4061223 > entire_4061223
# 1.4G    entire_4061223
with open("/Users/changwan.park/Documents/optimism/l2geth/entire_4061223") as f:
    for i, line in enumerate(f.readlines()):
        data = json.loads(line.strip())
        if len(data) == 1:
            assert (
                data["root"]
                == "0xda130177c0be61cd6c00cff0fc6a96f65fba892748637a1faa733241bc2eac3c"
            )
            continue  # skip root
        if "storage" in data:
            address = data["address"]
            assert address[:2] == "0x"
            correct_storage[address] = data["storage"]

predeploy_storage = dict()
# from tei
with open("goerli-genesis-l2.json", "r") as f:
    goerli_genesis_l2_alloc = json.loads(f.read())["alloc"]
    for address, value in goerli_genesis_l2_alloc.items():
        address = "0x" + address
        if "storage" in value:
            predeploy_storage[address] = value["storage"]


world_trie_storage = {}
world_trie = MerklePatriciaTrie(world_trie_storage, secure=True)
errored = []
# ./build/bin/geth --datadir=/Users/changwan.park/Downloads/goerli-bedrock-archive --nodiscover dump --iterative --incompletes 4061224 > everything_4061224
with open("/Users/changwan.park/Documents/op-geth/everything_4061224", "r") as f:
    for i, line in enumerate(f.readlines()):
        data = json.loads(line.strip())
        if len(data) == 1:
            assert (
                data["root"]
                == "0xbfe2b059bc76c33556870c292048f1d28c9d498462a02a3c7aadb6edf1c2d21c"
            )
            continue  # skip root
        key = data["key"]
        assert key in final_preimage
        address = final_preimage[key]
        if not address.startswith("0x"):
            address = "0x" + address
        balance = data["balance"]
        nonce = data["nonce"]
        root = data["root"]
        codeHash = data["codeHash"]
        code, storage = None, None
        if "code" in data:
            code = data["code"]
            if VALIDATE:
                validate_code(code, codeHash)
        else:
            # empty code
            assert (
                codeHash
                == "0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"
            )
        if "storage" in data:
            if address in [
                "0x4200000000000000000000000000000000000000",
                "0x4200000000000000000000000000000000000014",
            ]:
                final_storage = dict()
                if address in correct_storage:
                    final_storage |= correct_storage[address]
                if address in predeploy_storage:
                    for key, value in predeploy_storage[address].items():
                        final_storage[key] = value
                storage = final_storage
            elif address in [
                "0x4200000000000000000000000000000000000010",
                "0x4200000000000000000000000000000000000011",
                "0x4200000000000000000000000000000000000014",
                "0x420000000000000000000000000000000000000f",
                "0x4200000000000000000000000000000000000007",  # must remove value == 0 for this
            ]:
                storage = predeploy_storage[address]
            elif address == "0xdeaddeaddeaddeaddeaddeaddeaddeaddead0000":
                # manual recovery by tei, or parse core/alloc/optimism-goerli.json at erigon
                storage = {
                    "0x0000000000000000000000000000000000000000000000000000000000000003": "0x457468657200000000000000000000000000000000000000000000000000000a",
                    "0x0000000000000000000000000000000000000000000000000000000000000004": "0x4554480000000000000000000000000000000000000000000000000000000006",
                    "0x0000000000000000000000000000000000000000000000000000000000000006": "0x0000000000000000000000004200000000000000000000000000000000000010",
                }
            elif address == "0x4200000000000000000000000000000000000016":
                storage = hack_for_0x4200000000000000000000000000000000000016()
            else:
                # do not use storage data from bedrock
                # storage = data["storage"]
                if address in correct_storage:
                    storage = correct_storage[address]
                elif address in predeploy_storage:
                    storage = predeploy_storage[address]
                else:
                    assert False, "no storage"

            print(i, len(storage), address)

            # remove zero keys
            zero_keys = []
            for key, value in storage.items():
                if int(value, 16) == 0:  # int func does not care about 0x prefix
                    zero_keys.append(key)
            for key in zero_keys:
                storage.pop(key)

            if VALIDATE:
                try:
                    validate_trie(storage, root)
                except:
                    print("Errored")
                    errored.append((address, i))
                    print(errored)
        else:
            # empty storage
            assert (
                root
                == "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421"
            )
        account = create_account(balance, nonce, root, code, codeHash, storage)
        result[address] = account

        encoded_account = encode_account(account)
        world_trie.update(bytes.fromhex(address[2:]), encoded_account)

        print(f"{i}/127608")
        print(errored)


assert (
    "0x" + world_trie.root_hash().hex()
    == "0xbfe2b059bc76c33556870c292048f1d28c9d498462a02a3c7aadb6edf1c2d21c"
)

print(errored)

with open("alloc_everything_4061224_final.json", "w") as f:
    f.write(json.dumps(result))


# target: 4061224: state root: 0xbfe2b059bc76c33556870c292048f1d28c9d498462a02a3c7aadb6edf1c2d21c

# 88376: 0x4200000000000000000000000000000000000000: 0xb0bd311b46a2440f3eaf346b6c1ce588ed08712591822a258c5a1c4cf44cd0c9
#  12600 correct storage keys, 2 predeploy storage keys
#  no overlap
#  state root from bedrock archive:
#    4061223: 0xf7a75779601c9273910770be5ef0f43ce75e8a346a80c77e21e6d405c9630b2f
#    4061224: 0xeb5ebb45ba1ff8cbbb517a45e8328a65a32c0397ecf2e8ecec2547b32290ca7b
#  state root from legacy archive
#    4061223: 0xf7a75779601c9273910770be5ef0f43ce75e8a346a80c77e21e6d405c9630b2f
# 89634: 0x4200000000000000000000000000000000000014: 0xb34b5edb274d76ceb21d6efd719e42452a208b3428e5d6a946077a4434a9d564
#  2 correct storage keys, 2 predeploy storage keys
#  2 overlap
#  state root from bedrock archive
#    4061223: 0x0bc68ed24b9ab8fa90717ab4a1f184e030c32d1e51b25923c4e704bf26061812
#    4061224: 0xb28ec35af3e82ebf04802d04cbf86a830fd76fd507e10c0ef09668403fee4759
#  state root from legacy archive
#    4061223: 0x0bc68ed24b9ab8fa90717ab4a1f184e030c32d1e51b25923c4e704bf26061812

# challenge
# 41948: 0x4200000000000000000000000000000000000016: 0x542220b0147f4cc0e0156d993334777d699c312c2fe454f8b3fa338ed309f4a0
#  address only in predeploy storage. 3 keys
#  state root from bedrock archive:
#    4061223: 0x6e029d095cc4c7c62c08d62cfd5b4519cc3ca3d142f7c6a940e67821473a6e7d
#    4061224: 0x3ed105f39087db62a27c353e0f130d6c2df93e69aad54e11daffc4dbb9c34e9b
#  state root from legacy archive
#


# works by
"""
            if address in correct_storage:
                print("correct storage added")
                final_storage |= correct_storage[address]
            if address in predeploy_storage:
                print("predeploy storage added")
                for key, value in predeploy_storage[address].items():
                    final_storage[key] = value
"""

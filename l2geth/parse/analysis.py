import json
import pickle

from web3 import Web3


def analysis(filename, key):
    account_keys = []
    account_num_with_balance_positive = 0
    with open(filename, "r") as f:
        root = json.loads(f.readline().strip())["root"]
        for line in f.readlines():
            account_data = json.loads(line.strip())
            account_key = account_data[key]
            account_keys.append(account_key)
            if int(account_data["balance"]) > 0:
                account_num_with_balance_positive += 1

    assert len(account_keys) == len(set(account_keys))
    return set(account_keys)


def recovery(address_set):
    recovered_preimages = dict()
    for address in address_set:
        key = Web3.keccak(hexstr=address).hex()
        if key in bedrock_account_keys_4061224:
            recovered_preimages[key] = address
    return recovered_preimages


def predeploy_contracts():
    recovered_preimages = dict()
    start = 0x4200000000000000000000000000000000000000
    for i in range(0x10000):
        address = hex(start + i)
        key = Web3.keccak(hexstr=address).hex()
        recovered_preimages[key] = address

    start = 0xC0D3C0D3C0D3C0D3C0D3C0D3C0D3C0D3C0D30000
    for i in range(0x10000):
        address = hex(start + i)
        key = Web3.keccak(hexstr=address).hex()
        recovered_preimages[key] = address

    start = 0x6900000000000000000000000000000000000000
    for i in range(0x10000):
        address = hex(start + i)
        key = Web3.keccak(hexstr=address).hex()
        recovered_preimages[key] = address

    for i in range(256):
        address = hex(i)
        key = Web3.keccak(hexstr=address).hex()
        recovered_preimages[key] = address

    artifacts = [
        "0x14dc79964da2c08b23698b3d3cc7ca32193d9955",
        "0x15d34aaf54267db7d7c367839aaf71a00a2c6a65",
        "0x1cbd3b2770909d4e10f157cabc84c7264073c9ec",
        "0x23618e81e3f5cdf7f54c3d65f7fbc0abf5b21e8f",
        "0x2546bcd3c84621e976d8185a91a922ae77ecec30",
        "0x3c44cdddb6a900fa2b585dd299e03d12fa4293bc",
        "0x70997970c51812dc3a010c7d01b50e0d17dc79c8",
        "0x71562b71999873db5b286df957af199ec94617f7",
        "0x71be63f3384f5fb98995898a86b02fb2426c5788",
        "0x8626f6940e2eb28930efb4cef49b2d1f2c9c1199",
        "0x90f79bf6eb2c4f870365e785982e1f101e93b906",
        "0x976ea74026e726554db657fa54763abd0c3a0aa9",
        "0x9965507d1a55bcc2695c58ba16fb37d819b0a4dc",
        "0xa0ee7a142d267c1f36714e4a8f75612f20a79720",
        "0xbcd4042de499d14e55001ccbb24a551f3b954096",
        "0xbda5747bfd65f08deb54cb465eb87d40e51b197e",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30000",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30002",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30007",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3000f",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30010",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30011",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30012",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30013",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30014",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30015",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30016",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30017",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30018",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d30019",
        "0xc0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3c0d3001a",
        "0xcd3b766ccdd6ae721141f452c550ca635964ce71",
        "0xdd2fd4581271e230360230f9337d5c0430bf44c0",
        "0xde3829a23df1479438622a08a116e8eb3f620bb5",
        "0xdeaddeaddeaddeaddeaddeaddeaddeaddead0000",
        "0xdf3e18d64bc6a983f673ab319ccae4f1a57c7097",
        "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266",
        "0xfabb0ac9d68b0b445fb7357272ff202c5651694a",
    ]
    for address in artifacts:
        key = Web3.keccak(hexstr=address).hex()
        recovered_preimages[key] = address
    return recovered_preimages


def address_by_trace_search():
    address_from_trace = set()
    with open("out_merged", "rb") as f:
        address_from_trace = pickle.load(f)
    return address_from_trace


def address_by_trace_recovery_search():
    address_from_trace = set()
    with open("out_recovery_merged", "rb") as f:
        address_from_trace = pickle.load(f)
    return address_from_trace


def address_by_tx_txreceipt_search(filename):
    addresses = set()
    with open(filename) as f:
        for line in f.readlines():
            address = json.loads(line.strip())["address"]
            addresses.add(address)
    return addresses


def address_from_event_search():
    address_from_event = set()
    with open("/Users/changwan.park/Documents/op-geth/address_from_event", "rb") as f:
        address_from_event |= pickle.load(f)
    with open("/Users/changwan.park/Documents/op-geth/address_from_event_2", "rb") as f:
        address_from_event |= pickle.load(f)
    with open("/Users/changwan.park/Documents/op-geth/address_from_event_3", "rb") as f:
        address_from_event |= pickle.load(f)
    with open(
        "/Users/changwan.park/Documents/op-geth/address_from_event_refactored", "rb"
    ) as f:
        address_from_event |= pickle.load(f)
    return address_from_event


def address_optimism_l2geth_config_brute_search():
    cand_addresses = set()
    with open("/Users/changwan.park/Downloads/addrs.txt", "r") as f:
        for line in f.readlines():
            cand = line.strip()
            try:
                _ = int(cand, 16)
                cand_addresses.add(cand)
            except:
                continue
    return cand_addresses


def check(address, hashes):
    return Web3.keccak(hexstr=address).hex() in hashes


bedrock_account_keys_4061224 = analysis(
    "/Users/changwan.park/Documents/op-geth/bedrock_entire_4061224_iterative_nocode_nostorage",
    "key",
)
assert len(bedrock_account_keys_4061224) == 127608  # need to recover
legacy_account_addresses_4061223 = analysis(
    "/Users/changwan.park/Documents/optimism/l2geth/legacy_entire_4061223_iterative_nocode_nostorage",
    "address",
)
assert len(legacy_account_addresses_4061223) == 76117


union = set()
unionAddress = dict()
print("Unknown counts", len(bedrock_account_keys_4061224))


recovered_preimage_from_predeploy = predeploy_contracts()
print("Recovered using address from predeploy,", len(recovered_preimage_from_predeploy))
union |= set(recovered_preimage_from_predeploy.keys())
diff = bedrock_account_keys_4061224 - union
print("Unknown", len(diff))

address_optimism_l2geth_config_brute = address_optimism_l2geth_config_brute_search()
recovered_preimage_from_address_address_optimism_l2geth_config_brute = recovery(
    address_optimism_l2geth_config_brute
)
union |= set(
    recovered_preimage_from_address_address_optimism_l2geth_config_brute.keys()
)
print(
    "Recovered using address from l2geth config brute,",
    len(recovered_preimage_from_address_address_optimism_l2geth_config_brute),
)
diff = bedrock_account_keys_4061224 - union
print("Unknown", len(diff))

address_by_trace = address_by_trace_search()
recovered_preimages_from_trace = recovery(address_by_trace)
print("Recovered using address from trace,", len(recovered_preimages_from_trace))
union |= set(recovered_preimages_from_trace.keys())
diff = bedrock_account_keys_4061224 - union
print("Unknown", len(diff))

recovered_preimages_from_legacy = recovery(legacy_account_addresses_4061223)
print("Recovered using address from legacy,", len(recovered_preimages_from_legacy))
union |= set(recovered_preimages_from_legacy.keys())
diff = bedrock_account_keys_4061224 - union
print("Unknown", len(diff))

tx_txreceipt_search_address_set = address_by_tx_txreceipt_search(
    "/Users/changwan.park/Documents/optimism/l2geth/addresses_0_4061223"
)
recovered_preimages_from_tx_txreceipt_search = recovery(tx_txreceipt_search_address_set)
union |= set(recovered_preimages_from_tx_txreceipt_search.keys())
print(
    "Recovered using address from tx txreceipt search,",
    len(recovered_preimages_from_tx_txreceipt_search),
)
diff = bedrock_account_keys_4061224 - union
print("Unknown", len(diff))

address_from_event = address_from_event_search()
recovered_preimage_from_address_from_event = recovery(address_from_event)
union |= set(recovered_preimage_from_address_from_event.keys())
print(
    "Recovered using address from event,",
    len(recovered_preimage_from_address_from_event),
)
diff = bedrock_account_keys_4061224 - union
print("Unknown", len(diff))

address_by_trace_recovery = address_by_trace_recovery_search()
recovered_preimages_from_trace_recovery = recovery(address_by_trace_recovery)
print(
    "Recovered using address from trace recovery,",
    len(recovered_preimages_from_trace_recovery),
)
union |= set(recovered_preimages_from_trace_recovery.keys())
diff = bedrock_account_keys_4061224 - union
print("Unknown", len(diff))

address_by_selfdestruct_recovery = [
    "0x39ba31a33f673daa769904393554a4010bc13c0c",
    "0xe49aee3a194260826859fb70379bfbee4954172c",
]
recovered_address_by_selfdestruct_recovery = recovery(address_by_selfdestruct_recovery)
print(
    "Recovered using selfdestruct recovery,",
    len(recovered_address_by_selfdestruct_recovery),
)
union |= set(recovered_address_by_selfdestruct_recovery.keys())
diff = bedrock_account_keys_4061224 - union
print("Unknown", len(diff))

assert len(diff) == 0

unionAddress = recovered_preimages_from_trace
unionAddress |= recovered_preimages_from_legacy
unionAddress |= recovered_preimage_from_predeploy
unionAddress |= recovered_preimages_from_tx_txreceipt_search
unionAddress |= recovered_preimage_from_address_from_event
unionAddress |= recovered_preimage_from_address_address_optimism_l2geth_config_brute
unionAddress |= recovered_preimages_from_trace_recovery
unionAddress |= recovered_address_by_selfdestruct_recovery

result = dict()

for key in bedrock_account_keys_4061224:
    if key in unionAddress:
        result[key] = unionAddress[key]

every_address = []
for key, address in result.items():
    every_address.append(address)
assert len(set(every_address)) == 127608

with open("final_preimage.pickle", "wb") as f:
    pickle.dump(result, f)

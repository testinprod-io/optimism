import web3
from web3.middleware import geth_poa_middleware
import pickle

RPC = "http://localhost:8545"
w3 = web3.Web3(web3.HTTPProvider(RPC))
w3.middleware_onion.inject(geth_poa_middleware, layer=0)


transfer_topic = web3.Web3.keccak(text="Transfer(address,address,uint256)").hex()
approval_topic = web3.Web3.keccak(text="Approval(address,address,uint256)").hex()
deposit_topic = web3.Web3.keccak(text="Deposit(address,uint256)").hex()
withdrawal_topic = web3.Web3.keccak(text="Withdrawal(address,uint256)").hex()
mint_topic = web3.Web3.keccak(text="Mint(address,uint256)").hex()
withdrawalInitiated_topic = web3.Web3.keccak(
    text="WithdrawalInitiated(address,address,address,address,uint256,bytes)"
).hex()
depositFinalized_topic = web3.Web3.keccak(
    text="DepositFinalized(address,address,address,address,uint256,bytes)"
).hex()

target_topics = [
    transfer_topic,
    approval_topic,
    deposit_topic,
    withdrawal_topic,
    mint_topic,
    withdrawalInitiated_topic,
    depositFinalized_topic,
]


CHUNK = 100000


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

target_addresses = [0xDEADDEADDEADDEADDEADDEADDEADDEADDEAD0000]
target_addresses += [int(addr, 16) for addr in artifacts]
namespace = 0x6900000000000000000000000000000000000000
for i in range(0xF):
    target_addresses.append(namespace + i)
namespace = 0x4200000000000000000000000000000000000000
for i in range(0x20):
    target_addresses.append(namespace + i)
namespace = 0xC0D3C0D3C0D3C0D3C0D3C0D3C0D3C0D3C0D30000
for i in range(0xFF):
    target_addresses.append(namespace + i)

target_addresses = [web3.Web3.toChecksumAddress(hex(addr)) for addr in target_addresses]
target_addresses = set(target_addresses)


def parse(value):
    return value.lstrip(b"\x00").hex().zfill(40).lower()


def logic(address):
    fromBlock, toBlock = 0, CHUNK
    address_set = set()
    for _ in range(41):
        # print(fromBlock, toBlock)
        logs = w3.eth.get_logs(
            {
                "fromBlock": fromBlock,
                "toBlock": toBlock,
                "topics": [target_topics],
                "address": address,
            }
        )
        # print(f"total log cnt : {len(logs)}")
        for log in logs:
            # remove topic hash and leave indexed address
            values = log["topics"][1:]
            for value in values:
                parsed = parse(value)
                address_set.add(parsed)

        fromBlock += CHUNK
        toBlock += CHUNK

    print(f"addressSet len {len(address_set)}")
    return address_set


full_address_set = set()
for address in target_addresses:
    print(address)
    full_address_set |= logic(address)
    print(len(full_address_set))


with open("address_from_event_refactored", "wb") as f:
    pickle.dump(full_address_set, f)

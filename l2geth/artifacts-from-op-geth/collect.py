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

target_topics = [
    transfer_topic,
    approval_topic,
    deposit_topic,
    withdrawal_topic,
    mint_topic,
]

#depositFinalized_topic = web3.Web3.keccak(text="DepositFinalized(address,address,address,address,uint256,bytes)").hex()
#target_topics = [depositFinalized_topic]

withdrawalInitiated_topic = web3.Web3.keccak(text="WithdrawalInitiated(address,address,address,address,uint256,bytes)").hex()
target_topics = [withdrawalInitiated_topic]

def parse(value):
    return value.lstrip(b"\x00").hex().zfill(40).lower()


# WETH

# 0xDeadDeAddeAddEAddeadDEaDDEAdDeaDDeAD0000 DONE
# 0x4200000000000000000000000000000000000006 DONE
# 0x4200000000000000000000000000000000000010
# 0x4200000000000000000000000000000000000007

CHUNK = 100000
fromBlock, toBlock = 0, CHUNK
addressSet = set()
for i in range(41):
    print(fromBlock, toBlock)
    logs = w3.eth.get_logs(
        {
            "fromBlock": fromBlock,
            "toBlock": toBlock,
            "topics": [target_topics],
            "address": "0x4200000000000000000000000000000000000010",
        }
    )
    print(f"total log cnt : {len(logs)}")
    for log in logs:
        # remove topic hash and leave indexed address
        values = log["topics"][1:]
        for value in values:
            parsed = parse(value)
            addressSet.add(parsed)
    print(f"addressSet len {len(addressSet)}")

    fromBlock += CHUNK
    toBlock += CHUNK

# with open("address_from_event_3", "wb") as f:
#     pickle.dump(addressSet, f)

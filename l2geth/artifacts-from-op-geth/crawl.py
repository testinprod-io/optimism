import requests
import os 
import time
import pickle
# cmd = "wget https://goerli-optimism.etherscan.io/accounts/{idx}?ps=100"
# for i in range(1, 101):
#     os.system(cmd.format(idx=i))
#     time.sleep(0.5)

addressSet = set()
for i in range(1, 101):
    with open("raw_data/{idx}?ps=100".format(idx=i)) as f:
        data = f.read()
        data = data[data.find("/address/") - len("/address/"):].split("</tbody>")[0]
        data = data.split("/address/")
        data = [d[:42] for d in data][1:101]
        assert len(data) == 100
        for address in data:
            assert address[:2] == "0x" and len(address) == 42
            addressSet.add(address)


with open("address_from_etherscan", "wb") as f:
    pickle.dump(addressSet, f)


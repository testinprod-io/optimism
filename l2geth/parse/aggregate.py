import os
import pickle
from typing import List, Set

path = "output"
if not os.path.exists(path):
    print("output directory nonexistent")
    exit()
os.chdir(path)


full_address_set = set()
full_error_block_number = set()
full_missed = []
for filename in os.listdir("./"):
    with open(filename, "rb") as f:
        data = pickle.load(f)
        address_set: Set = data["address_set"]
        error_block_number: List = data["error_block_number"]
        missed: List = data["missed"]
        full_address_set |= address_set
        full_error_block_number |= set(error_block_number)
        full_missed.extend(missed)


print(full_error_block_number)
print(missed)
print(len(full_address_set))

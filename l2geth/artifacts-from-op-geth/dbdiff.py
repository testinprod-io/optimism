import json
from collections import defaultdict

keys_4061223 = set()
storage_roots_4061223 = set()

# legacy_entire_4061223_iterative_nocode_nostorage
with open("bedrock_entire_4061223_iterative_nocode_nostorage", "r") as f:
    for i, line in enumerate(f.readlines()):
        data = json.loads(line.strip())
        if len(data) == 1:
            continue  # skip root
        keys_4061223.add(data["key"])
        storage_roots_4061223.add(data["root"])


storage_roots = defaultdict(int)
storage_roots_data = defaultdict(list)
with open("bedrock_entire_4061224_iterative_nocode_nostorage", "r") as f:
    for i, line in enumerate(f.readlines()):
        data = json.loads(line.strip())
        if len(data) == 1:
            continue  # skip root
        key = data["key"]
        if key not in keys_4061223:
            storage_roots[data["root"]] += 1
            storage_roots_data[data["root"]].append(data)

# print(len(storage_roots))
# print(storage_roots)

# 0x2527617e608e162462d4098c8338838e3b87b562cd2096c05c7078777993827b


unknown_keys = []
for root in storage_roots:
    if root not in storage_roots_4061223:
        print(root, storage_roots[root])
        # print(storage_roots_data[root][:3])
        for d in storage_roots_data[root]:
            if root != "0x2527617e608e162462d4098c8338838e3b87b562cd2096c05c7078777993827b":
                unknown_keys.append(d["key"])


print(len(unknown_keys))
print(unknown_keys)

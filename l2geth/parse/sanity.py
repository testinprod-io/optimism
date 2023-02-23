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

count_4096 = 0
count_64 = 0
for filename in os.listdir("./"):
    [start, end] = [int(d) for d in filename.split("_")[-2:]]
    diff = end - start
    if diff == 4096 or diff == 4095:
        count_4096 += 1
    with open(filename, "rb") as f:
        data = pickle.load(f)
        address_set: Set = data["address_set"]
        error_block_number: List = data["error_block_number"]
        missed: List = data["missed"]
        full_address_set |= address_set
        full_error_block_number |= set(error_block_number)
        full_missed.extend(missed)

print(full_missed)

data = """
preimage-attack
–
Feb 21st
preimage-attacker
APP 2:06 PM
0x20f9b1, 0x20f9c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:01 AM
0x2a8241, 0x2a8280 failed
:meow_party:1 reaction
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:40 AM
0xf0001, 0xf0040 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:39 AM
0xf0081, 0xf00c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:38 AM
0xf0081, 0xf00c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:38 AM
0xf00c1, 0xf0100 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:36 AM
0x97281, 0x972c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:35 AM
0x914c1, 0x91500 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:19 AM
0x582c1, 0x58300 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:18 AM
0x55101, 0x55140 failed
preimage-attack
–
Feb 20th
preimage-attacker
APP 9:11 PM
0x3dbe40, 0x3dbf40 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 4:53 AM
0x2cef41, 0x2cef80 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 4:47 AM
0x2cdc01, 0x2cdc40 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 4:45 AM
0x2cd741, 0x2cd780 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 4:30 AM
0x2ca5c1, 0x2ca600 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 4:21 AM
0x2c7681, 0x2c76c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 4:09 AM
0x2c0ec1, 0x2c0f00 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:41 AM
0x2b9341, 0x2b9380 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:39 AM
0x2b8841, 0x2b8880 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:31 AM
0x2b57c1, 0x2b5800 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:19 AM
0x2af5c1, 0x2af600 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:06 AM
0x2ab381, 0x2ab3c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:06 AM
0x2aae41, 0x2aae80 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 2:08 AM
0x295a81, 0x295ac0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 2:03 PM
0x20fb41, 0x20fb80 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 2:01 PM
0x20fb01, 0x20fb40 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 1:52 PM
0x20fb01, 0x20fb40 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 1:46 PM
0x20fc41, 0x20fc80 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 1:56 PM
0x2ed441, 0x2ed480 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 1:51 PM
0x20fb41, 0x20fb80 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 1:44 PM
0x20fb81, 0x20fbc0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 1:59 PM
0x20f981, 0x20f9c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 1:54 PM
0x20f981, 0x20f9c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 1:49 PM
0x210701, 0x210740 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 1:47 PM
0x210501, 0x210540 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 1:47 PM
0x229601, 0x229640 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:55 AM
0x20fb81, 0x20fbc0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:47 AM
0x20fb01, 0x20fb40 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:48 AM
0x20fb41, 0x20fb80 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:43 AM
0x2ed441, 0x2ed480 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 1:50 PM
0x2297c1, 0x229800 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:52 AM
0x210501, 0x210540 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:50 AM
0x210701, 0x210740 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:39 AM
0xf0001, 0xf0040 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:39 AM
0xf0041, 0xf0080 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:39 AM
0x9b681, 0x9b6c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:38 AM
0xf0041, 0xf0080 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:38 AM
0xf0141, 0xf0180 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:31 AM
0x86ec1, 0x86f00 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:27 AM
0x78e81, 0x78ec0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:29 AM
0x7f001, 0x7f040 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:29 AM
0x7f001, 0x7f040 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:21 AM
0x5e401, 0x5e440 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:19 AM
0x57d01, 0x57d40 failed
preimage-attack
–
Feb 20th
preimage-attacker
APP 10:22 PM
0x3dc140, 0x3dc240 failed
preimage-attack
–
Feb 20th
preimage-attacker
APP 9:15 PM
0x3dc140, 0x3dc240 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:53 AM
0x20fc41, 0x20fc80 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:45 AM
0x20f981, 0x20f9c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 4:07 AM
0x2c0801, 0x2c0840 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 4:02 AM
0x2bf441, 0x2bf480 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:50 AM
0x2bbd01, 0x2bbd40 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:06 AM
0x2ab681, 0x2ab6c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:06 AM
0x2ab801, 0x2ab840 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 2:09 AM
0x295f81, 0x295fc0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:40 AM
0xf0041, 0xf0080 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:38 AM
0xf0181, 0xf01c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:38 AM
0xf0101, 0xf0140 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:22 AM
0x622c1, 0x62300 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:40 AM
0xf0081, 0xf00c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:38 AM
0xf0001, 0xf0040 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 12:22 AM
0x61c81, 0x61cc0 failed
preimage-attack
–
Feb 20th
preimage-attacker
APP 10:18 PM
0x3dbe40, 0x3dbf40 failed
preimage-attack
–
Feb 20th
preimage-attacker
APP 10:20 PM
0x3dc040, 0x3dc140 failed
preimage-attack
–
Feb 20th
preimage-attacker
APP 10:17 PM
0x3dbc40, 0x3dbd40 failed
preimage-attack
–
Feb 20th
preimage-attacker
APP 10:13 PM
0x3db740, 0x3db840 failed
preimage-attack
–
Feb 20th
preimage-attacker
APP 9:13 PM
0x3dc040, 0x3dc140 failed
preimage-attack
–
Feb 20th
preimage-attacker
APP 9:09 PM
0x3dbc40, 0x3dbd40 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 4:03 AM
0x2bf881, 0x2bf8c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:33 AM
0x2b6fc1, 0x2b7000 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:32 AM
0x2b64c1, 0x2b6500 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:26 AM
0x2b1181, 0x2b11c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:08 AM
0x2acc41, 0x2acc80 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:05 AM
0x2a9e41, 0x2a9e80 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 2:44 AM
0x2a3901, 0x2a3940 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 2:04 AM
0x294dc1, 0x294e00 failed
1 reply
preimage-attack
–
Feb 21st
preimage-attacker
APP 1:58 AM
0x293601, 0x293640 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 1:15 AM
0x287c41, 0x287c80 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:52 AM
0x2bc7c1, 0x2bc800 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:30 AM
0x2b5081, 0x2b50c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 3:22 AM
0x2b01c1, 0x2b0200 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 2:25 AM
0x29a281, 0x29a2c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:33 AM
0x20f981, 0x20f9c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:13 AM
0x20fb81, 0x20fbc0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:12 AM
0x20fb41, 0x20fb80 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:15 AM
0x20fc41, 0x20fc80 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:10 AM
0x20fb01, 0x20fb40 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:08 AM
0x20f981, 0x20f9c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:35 AM
0x2ed441, 0x2ed480 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:27 AM
0x20fb41, 0x20fb80 failed
preimage-attack
–
Feb 21st
View in channel
preimage-attacker
APP 11:22 AM
0x210701, 0x210740 failed
preimage-attacker
APP 11:45 AM
0x20f981, 0x20f9c0 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:31 AM
0x20fb01, 0x20fb40 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:19 AM
0x210501, 0x210540 failed
preimage-attack
–
Feb 21st
preimage-attacker
APP 11:06 AM
0x2ed441, 0x2ed480 failed
"""


from ast import literal_eval

missed_2 = []  # from slack
for line in data.split("\n"):
    if "failed" in line:
        s, e = literal_eval(line.strip().rstrip(" failed"))
        print(e - s)
        missed_2.append((s, e))

exit()
print(missed_2)
print("slack", len(missed_2))

wtf = []
for miss in missed_2:
    if miss not in full_missed:  # from s3
        print(miss)
        wtf.append(miss)
print(len(full_missed))
print(wtf)

# 0x3dc040


# 2799233, 2799296

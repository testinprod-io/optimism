import pickle
import random
import sys
from typing import Dict

import requests

sys.setrecursionlimit(1500)


URL = "http://localhost:8545"

start = 0x290301
end = start + 0x100 - 1
DEFAULT_TIMEOUT = 120
EXEC_TIMEOUT = "60s"
MAX_BLOCK_NUM = 4061223

address_set = set()
error_block_number = []


def get_body(start: int, end: int) -> Dict:
    body = {
        "jsonrpc": "2.0",
        "method": "debug_traceAddresses",
        "params": [
            hex(start),
            hex(end),
            {"tracer": "myTracer", "timeout": EXEC_TIMEOUT},
        ],
        "id": 1,
    }
    return body


def traverse(result):
    if "from" in result:
        address_set.add(result["from"])
    if "to" in result:
        address_set.add(result["to"])
    if "calls" in result:
        for subcall in result["calls"]:
            traverse(subcall)


def trace(start: int, end: int):
    print("start:", hex(start), "end:", hex(end))
    while True:
        timeout = DEFAULT_TIMEOUT
        failure_count = 0
        try:
            r = requests.get(URL, json=get_body(start, end), timeout=timeout)
            assert r.status_code == 200
            print(r.elapsed.total_seconds())
            break
        except requests.exceptions.HTTPError as err:
            print("HTTP Error:", err)
            failure_count += 1
        except requests.exceptions.ConnectionError as err:
            print("Connection Error:", err)
            failure_count += 1
        except requests.exceptions.Timeout as err:
            print("Timeout Error:", err)
            failure_count += 1
            timeout += 10
        except requests.exceptions.RequestException as err:
            print("Unkwown Error:", err)
            return
        if failure_count == 3:
            return

    results = r.json()["result"]

    for i, result in enumerate(results):
        # {'error': 'execution timeout'}
        if "error" in result:
            error_block_number.append(i + start)
        if "result" in result:
            traverse(result["result"])

    with open("output.pickle", "wb") as f:
        pickle.dump(
            {"address_set": address_set, "error_block_number": error_block_number}, f
        )

    print(error_block_number)
    print(len(address_set))



for _ in range(10):
    start = random.randint(3000000, MAX_BLOCK_NUM - 100)
    end = start + 0x100 - 1
    trace(start, end)


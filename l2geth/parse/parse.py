"""
Usage: python3.10 parse.py [slack token] [start_number] [end_number]
example: python3.10 parse.py [REDACTED] 4044608 4044863

The script will trace blocks between [start_number] and [end_number] inclusively
"""

import pickle
import sys
import time
from typing import Dict

import boto3
import requests
import slack_sdk
from botocore.exceptions import ClientError
from slack_sdk import WebClient

sys.setrecursionlimit(1500)

slack_token = sys.argv[1]
slack_client = WebClient(slack_token)
s3_client = boto3.client("s3")

URL = "http://localhost:8545"
DEFAULT_TIMEOUT = 120
EXEC_TIMEOUT = "60s"
MAX_BLOCK_NUM = 4061223

address_set = set()
error_block_number = []
missed = []


def get_filename(start: int, end: int) -> str:
    return f"preimage_{hex(start)}_{hex(end)}_{start}_{end}"


def send_message(text: str):
    time.sleep(0.1)
    try:
        r = slack_client.chat_postMessage(channel="preimage-attack", text=text)
    except slack_sdk.errors.SlackApiError as err:
        print("SlackApiError", err)
    except slack_sdk.errors.SlackClientError as err:
        print("SlackClientError", err)
    except Exception as err:
        print("Unkwown Error:", err)


def save_artifact(filename: str):
    print(f"uploading {filename}")
    time.sleep(0.1)
    try:
        response = s3_client.upload_file(
            filename, "preimage-recovery", "output2/" + filename
        )
        send_message(f"{filename} saved")
    except ClientError as err:
        print("Client Error", err)
    except Exception as err:
        print("Unknown Error:", err)


def save_file(filename: str):
    print(f"saving {filename}")
    with open(filename, "wb") as f:
        pickle.dump(
            {
                "address_set": address_set,
                "error_block_number": error_block_number,
                "missed": missed,
            },
            f,
        )
    # read with pickle.load(f)


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


def traverse(result, block_tag):
    if "from" in result:
        address_set.add(result["from"])
    if "to" in result:
        address_set.add(result["to"])
    if "calls" in result:
        for subcall in result["calls"]:
            traverse(subcall, block_tag)
    if "type" in result and result['type'] == 'SELFDESTRUCT':
        send_message(f"Found SELFDESTRUCT {block_tag}")


def trace(start: int, end: int) -> bool:
    print("start:", hex(start), "end:", hex(end))
    failure_count = 0
    timeout = DEFAULT_TIMEOUT
    while True:
        r = None
        try:
            r = requests.get(URL, json=get_body(start, end), timeout=timeout)
            assert r.status_code == 200
            print(r.elapsed.total_seconds())
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
            print("Unknown Error:", err)
            return False

        try:
            if r is not None:
                results = r.json()["result"]
                break
        except Exception as err:
            print("Json decode error", err)
            failure_count += 1

        if failure_count == 3:
            return False

    for i, result in enumerate(results):
        # {'error': 'execution timeout'}
        if "error" in result:
            error_block_number.append(i + start)
        if "result" in result:
            traverse(result["result"], f'{start}_{end}')
        if "error" not in result and "result" not in result:
            print("Error: error key and result key not present at response")
            return False

    print(len(address_set))
    return True


start = int(sys.argv[2])  # 4040000
end = int(sys.argv[3])  # 4061223

# reports every 0x1000
# each request handles 0x20
REPORT_INTERVAL = 0x1000
CLAMP = 0x40
# higher CLAMP, more likely to timeout

for s in range(start, end, REPORT_INTERVAL):
    e = min(end, s + REPORT_INTERVAL)
    filename = get_filename(s, e)
    send_message(f"{filename} START")
    for ss in range(s, e + 1, CLAMP):
        ee = min(end, ss + CLAMP - 1)
        print(ss, ee)
        if not trace(ss, ee):
            send_message(f"{hex(ss)}, {hex(ee)} failed")
            missed.append((ss, ee))
    save_file(filename)
    save_artifact(filename)
    send_message(f"{filename} END")
    send_message(f"{filename} has {len(address_set)} preimages")
    address_set = set()
    error_block_number = []
    missed = []

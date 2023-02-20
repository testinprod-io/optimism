import boto3
import os

path = "output"
if not os.path.exists(path):
    os.makedirs(path)
os.chdir(path)

s3 = boto3.resource("s3")
s3_client = boto3.client("s3")

BUCKET_NAME = "preimage-recovery"
PREFIX = "output/"
bucket = s3.Bucket(BUCKET_NAME)
for name in bucket.objects.all():
    if not name.key.startswith(PREFIX) or name.key == PREFIX:
        continue
    print(f"Download: {name.key}")
    s3_client.download_file(BUCKET_NAME, name.key, str(name.key)[len(PREFIX):])

# Span Batch Tester

WIP but you can get the general overview

Data flow: `Frame -> Channel -> BatchV1s -> SpanBatch`

Compare BatchV1s(BatchV1 list) and SpanBatch.

## Example Usage

We need [batch_decoder](../batch_decoder/) to obtain channels.

`batch_decoder fetch`: Read channel frames:
```sh
./batch_decoder fetch --l1=[L1_RPC] --start=17760000 --end=17770000 --inbox=0xff00000000000000000000000000000000000010 --sender=0x6887246668a3b87F54DeB3b94Ba47a6f63F32985 --concurrent-requests=25
```

`batch_decoder reassemble`: Reassemble channel frames to channels:
```sh
./batch_decoder reassemble --inbox=0xff00000000000000000000000000000000000010 --in=/tmp/batch_decoder/transactions_cache --out=/tmp/batch_decoder/channel_cache
```

`span_batch_tester convert`: Convert channels with v0 batches to span batch.
```sh
./span_batch_tester convert --in=/tmp/batch_decoder/channel_cache --l2=[L2_RPC] --out=/tmp/span_batch_tester/span_batch_cache --genesis-timestamp=[genesis_timestamp]
```

`span_batch_tester analyze`: Analyze channels with v0 batches and its corresponding span batch.
```sh
./span_batch_tester analyze --in-channel=/tmp/batch_decoder/channel_cache --in-span-batch=/tmp/span_batch_tester/span_batch_cache --out=/tmp/span_batch_tester/result
```

Example logs(on mainnet):

```
[1/80] Channel ID: 00b9f5ad2cc4bda91e9d83a1149b91fc, L1SizeReductionPercentage: 2.741022 %
[2/80] Channel ID: 0180be8f09ef9ba8c8f34da4de99754c, L1SizeReductionPercentage: 2.640372 %
[3/80] Channel ID: 019865185fb821c3500ec92bf18cdf1f, L1SizeReductionPercentage: 2.892947 %
[4/80] Channel ID: 01a74f320aab8a35150d5d1d71e42c13, L1SizeReductionPercentage: 3.762513 %
[5/80] Channel ID: 022ac44dbfb8756dc8918366a8b7cee4, L1SizeReductionPercentage: 2.969038 %
[6/80] Channel ID: 0628f6c8090a5bd0e1a36a2294d54f4b, L1SizeReductionPercentage: 5.704166 %
[7/80] Channel ID: 06d33430ef7a25fdf16513d0830ed1d8, L1SizeReductionPercentage: 3.452649 %
[8/80] Channel ID: 091d4dc2bd07eb9043cc451bb6128b50, L1SizeReductionPercentage: 3.388687 %
[9/80] Channel ID: 09a455ff8f48fd3c4c4c119844d14e23, L1SizeReductionPercentage: 3.095162 %
[10/80] Channel ID: 0d54213c48e51c52ff01bad07e67aca0, L1SizeReductionPercentage: 2.956510 %
...
````

Example logs(on goerli):

```
[1/143] Channel ID: 0012531bbbd7f87b3f5dc39b9ffa1b39, L1SizeReductionPercentage: 1.551085 %
[2/143] Channel ID: 001d0b74e42e5748c2e151503de38c6a, L1SizeReductionPercentage: 14.540396 %
[3/143] Channel ID: 0028b671a15b759e6f0a162c36bcd3c9, L1SizeReductionPercentage: 1.143876 %
[4/143] Channel ID: 002d6753f1ecc4dac5ece135d5ad30c4, L1SizeReductionPercentage: 1.291216 %
[5/143] Channel ID: 006442c4c387b4e5abfbf1b5868cc660, L1SizeReductionPercentage: 1.071292 %
[6/143] Channel ID: 00a833117462681c9b3679c27ad614dc, L1SizeReductionPercentage: 0.896322 %
[7/143] Channel ID: 00ccb5d85a6f9063d2a84db65c41f03f, L1SizeReductionPercentage: 1.147978 %
[8/143] Channel ID: 00d652b37b72966598471c5ced5b61ab, L1SizeReductionPercentage: 1.529358 %
[9/143] Channel ID: 00dce9e162acb73ccfee63eeefb9eadd, L1SizeReductionPercentage: 0.701666 %
[10/143] Channel ID: 010e97dedbaba821e6c53d36b97d0a2b, L1SizeReductionPercentage: 2.392510 %
[11/143] Channel ID: 01144a2232b99afb4548ae86bffee31f, L1SizeReductionPercentage: 18.849324 %
[12/143] Channel ID: 0130423b23897cccc4c83490c57d6bc8, L1SizeReductionPercentage: 21.818182 %
[13/143] Channel ID: 01378731f9c58ac26cd457a51107a1aa, L1SizeReductionPercentage: 14.045595 %
[14/143] Channel ID: 0157f7cb58119647361444bf6bb9ae57, L1SizeReductionPercentage: 0.721008 %
...
```

Percentage spike is strange and only occurs in goerli. Need to sanity check.


Example output of result json. `091d4dc2bd07eb9043cc451bb6128b50` is channel ID:
```
➜  result jq . 091d4dc2bd07eb9043cc451bb6128b50.json
{
  "FrameCount": 10,
  "BatchV1sCompressedSize": 1134156,
  "BatchV1sUncompressedSize": 3408969,
  "BatchV1sCompressionRatio": 0.3326976572682239,
  "SpanBatchCompressedSize": 1095723,
  "SpanBatchUncompressedSize": 3368258,
  "SpanBatchCompressionRatio": 0.32530851259018756,
  "L1SizeReductionPercentage": 3.3886872705342075,
  "L1StartNum": 17766518,
  "L1EndNum": 17766567,
  "L1BlockCount": 50,
  "L2StartNum": 107322975,
  "L2EndNum": 107323274,
  "L2BlockCount": 300
}
```

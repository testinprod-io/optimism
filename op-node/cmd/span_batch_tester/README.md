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

`span_batch_tester fetch`: Read L2 blocks and store in the form ov v0 batches.
```sh
./span_batch_tester fetch --l2=[L2_RPC] --out=/tmp/span_batch_tester/batches_v0_cache --start=13630000 --end=13631000 --concurrent-requests=100
```

`span_batch_tester merge`: Merge consecutive v0 batches to span batches.
```sh
./span_batch_tester merge --start=13630000 --end=13631000 --l2=[L2_RPC] --genesis-timestamp=1673550516  --in=/tmp/span_batch_tester/batches_v0_cache --out=/tmp/span_batch_tester/merge_result
```

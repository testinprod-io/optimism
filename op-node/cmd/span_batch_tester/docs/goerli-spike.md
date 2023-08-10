## Goerli Spike

This document will be removed, and also be reorganized. Only for temporary information transfer.

Number of channels per network will be normalized.

## Problem Statement

Define `CompressedReductionPercent := 100 - 100 * (SpanBatchCompressedSize / BatchV1sCompressedSize) [%]`

Lets define compression _spike_ occurred when `CompressedReductionPercent > 7`.

We inspected 143 channels in goerli. 47 of channels experienced a spike(32.8 %).

We inspected 80 channels in mainnet. 0 of channels experienced a spike(0 %).

Statement: **What caused this spike?**

## Analysis

Lets define metrics.
- `SpanBatchMetadataSize`: byte size of span batch except tx related field
- `SpanBatchTxSize`: byte size of span batch of tx related field(`TxDataHeaders`, `TxHeaders`, `TxSigs`)
- `BatchV1sMetadataSize`: byte size of batchV1 arrays except tx related field
- `BatchV1sTxSize`: byte size of batchV1 arrays of tx related field(`Transactions`)

So span batch size == `SpanBatchMetadataSize + SpanBatchTxSize`, and batchV1s size == `BatchV1sMetadataSize + BatchV1sTxSize`.

The effect of using span batch will be maximized when `BatchV1sMetadataSize` is relatively larger than `BatchV1sTxSize`, this is because span batch reduces `BatchV1sMetadataSize`(span batch also reduces `BatchV1sTxSize`, by removing RLP headers etc, but negligible compared to the reduction of metadata size).

Lets sample two channels from goerli. Some fields of results are omitted.

No spike: `jq . 091a12852b12764ed7ca30a0acc421e0.json`
```
A = {
  "BatchV1sCompressedSize": 181131,
  "BatchV1sUncompressedSize": 270707,
  "SpanBatchCompressedSize": 179434,
  "SpanBatchUncompressedSize": 268319,
  "BatchV1sMetadataSize": 2457,
  "BatchV1sTxSize": 268250,
  "SpanBatchMetadataSize": 83,
  "SpanBatchTxSize": 268236,
  "CompressedReductionPercent": 0.9368909794568614,
  "UncompressedSizeReductionPercent": 0.8821345587664897,
  "L2TxCount": 25,
  "L2BlockCount": 30,
}
```

Spike: `jq . 0536a14474bd1c1ce5cf6bb9b9afff24.json`
```
B = {
  "BatchV1sCompressedSize": 6048,
  "BatchV1sUncompressedSize": 13394,
  "SpanBatchCompressedSize": 4571,
  "SpanBatchUncompressedSize": 11021,
  "BatchV1sMetadataSize": 2439,
  "BatchV1sTxSize": 10955,
  "SpanBatchMetadataSize": 83,
  "SpanBatchTxSize": 10938,
  "CompressedReductionPercent": 24.42129629629629,
  "UncompressedSizeReductionPercent": 17.716888158877108,
  "L2TxCount": 22,
  "L2BlockCount": 30,
}
```

Although span batch reduced metadata size a lot, from `BatchV1sMetadataSize` to `SpanBatchMetadataSize`, reduction of tx size, from `BatchV1sTxSize` to `SpanBatchTxSize` is negligible. This means span batch has more effect when tx size is relatively small compared to metadata size.

`BatchV1sTxSize / (BatchV1sMetadataSize + BatchV1sTxSize) * 100`
- A: `99.09 = 268250 / (2457 + 268250) * 100`
- B: `81.79 = 10955 / (2439 + 10955) * 100`

Channel B has relatively smaller tx size compared to its metadata. Span batch reduces size of metadata, so metadata size reduction effect will be larger in B, leading to a spike.

## Conclusion

Some goerli channels(32.8 %) had relatively small tx sizes compared to its metadata, leading boost of effect of using span batch, leading to a spike. This behavior depends on tx size distribution per channel.

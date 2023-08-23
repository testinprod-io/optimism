## Span batch format

Note that span-batches, unlike previous V0 batches,
encode *a range of consecutive* L2 blocks at the same time.

Introduce version `1` to the [batch-format](./derivation.md#batch-format) table:

| `batch_version` | `content`           |
|-----------------|---------------------|
| 1               | `prefix ++ payload` |

Notation:
- `++`: concatenation of byte-strings
- `span_start`: first L2 block in the span
- `span_end`: last L2 block in the span
- `uvarint`: unsigned Base128 varint, as defined in [protobuf spec]
- `rlp_encode`: a function that encodes a batch according to the [RLP format], and `[x, y, z]` denotes a list containing items `x`, `y` and `z`

[protobuf spec]: https://protobuf.dev/programming-guides/encoding/#varints

[RLP format]: https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp/

Where:

- `prefix = rel_timestamp ++ l1_origin_num ++ parent_check ++ l1_origin_check`
  - `rel_timestamp`: relative time since genesis, i.e. `span_start.timestamp - config.genesis.timestamp`.
  - `l1_origin_num`: `uvarint` number of l1 origin number.
  - `parent_check`: first 20 bytes of parent hash, i.e. `span_start.parent_hash[:20]`.
  - `l1_origin_check`: to ensure the intended L1 origins of this span of
        L2 blocks are consistent with the L1 chain, the blockhash of the last L1 origin is referenced.
        The hash is truncated to 20 bytes for efficiency, i.e. `span_end.l1_origin.hash[:20]`.
- `payload = block_count ++ origin_bits ++ block_tx_counts ++ txs`:
  - `block_count`: `uvarint` number of L2 blocks.
  - `origin_bits`: bitlist of `block_count` bits, right-padded to a multiple of 8 bits:
    1 bit per L2 block, indicating if the L1 origin changed this L2 block.
  - `block_tx_counts`: for each block, a `uvarint` of `len(block.transactions)`.
  - `txs`: L2 transactions which is reorganized and encoded as below.
- `txs = contract_creation_bits ++ y_parity_bits ++ tx_sigs ++ tx_nonces ++ tx_gases ++ tx_tos ++ tx_datas`
  - `contract_creation_bits`: bit list of `sum(block_tx_counts)` bits, right-padded to a multiple of 8 bits, 1 bit per L2 transactions, indicating if transaction is a contract creation transaction.
  - `y_parity_bits`: bit list of `sum(block_tx_counts)` bits, right-padded to a multiple of 8 bits, 1 bit per L2 transactions, indicating the y parity value when recovering transaction sender address.
  - `tx_sigs`: concatenated list of transaction signatures
    - `r` is encoded as big-endian `uint256`
    - `s` is encoded as big-endian `uint256`
  - `tx_nonces`: concatenated list of `uvarint` of `nonce` field.
  - `tx_gases`:  concatenated list of `uvarint` of gas limits.
    - `legacy`: `gasLimit`
    - `1`: ([EIP-2930]): `gasLimit`
    - `2`: ([EIP-1559]): `gas_limit`
  - `tx_tos`: concatenated list of `to` field. `to` field in contract creation transaction will be `nil` and ignored.
  - `tx_datas`: concatenated list of variable length rlp encoded data follwing [EIP-2718] encoded format using `TransactionType`.
    - `legacy`: `rlp_encode(value, gasPrice, data)`
    - `1`: ([EIP-2930]): `0x01 ++ rlp_encode(value, gasPrice, data, accessList)`
    - `2`: ([EIP-1559]): `0x02 ++ rlp_encode(value, max_priority_fee_per_gas, max_fee_per_gas, data, access_list)`

[EIP-2718]: https://eips.ethereum.org/EIPS/eip-2718

[EIP-2930]: https://eips.ethereum.org/EIPS/eip-2930

[EIP-1559]: https://eips.ethereum.org/EIPS/eip-1559

### Optimization Strategy

#### `tx_data_headers` Removal

We do not need to store length per each `tx_datas` elements even if those are variable length, because the elements itself is RLP encoded, containing their length in RLP prefix.

#### `Chain ID` Removal

Every transaction has chain id. We do not need to include chain id in span batch because L2 already knows its chain id, and use its own value for processing span batches while derivation.

#### Reorganization of constant length transaction fields

`signature`, `nonce`, `gaslimit`, `to` field are constant size, so these were split up completely and are grouped into individual arrays. This adds more complexity, but organizes data for improved compression.

#### RLP encoding for variable length fields

Further size optimization can be done by customly packing variable length fields, such as `access_list`. However doing this will introduce much more code complexity, comparing to benefitting by size reduction.

Our goal is to find the sweet spot on code complexity - span batch size tradeoff. I decided that using RLP for all variable length fields will be the best option, not risking codebase with gnarly custom encoding/decoding implementations.

#### Store `y_parity` instead of `v`

For legacy type transactions, `v = 2 * ChainID + y_parity`. For other types of transactions, `v = y_parity`. We may only store `y_parity`, which is single bit per L2 transction.

This optimization will benefit more when ratio between number of legacy type transactions over number of transactions excluding deposit tx is higher. Deposit transactions are excluded in batches and are never written at L1 so excluded while analyzing.

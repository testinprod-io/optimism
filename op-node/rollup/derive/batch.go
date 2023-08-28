package derive

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sort"
	"sync"

	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// Batch format
// first byte is type followed by bytestring.
//
// BatchV1Type := 0
// batchV1 := BatchV1Type ++ RLP([epoch, timestamp, transaction_list]
//
// An empty input is not a valid batch.
//
// Note: the type system is based on L1 typed transactions.
// BatchV2Type := 1
// batchV2 := BatchV2Type ++ prefix ++ payload
// prefix := rel_timestamp ++ l1_origin_num ++ parent_check ++ l1_origin_check
// payload := block_count ++ origin_bits ++ block_tx_counts ++ txs

// encodeBufferPool holds temporary encoder buffers for batch encoding
var encodeBufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

const (
	BatchV1Type = iota
	BatchV2Type
)

const (
	BatchV2TxsV1Type = iota
	BatchV2TxsV2Type
	BatchV2TxsV3Type
)

// adjust this for trying different tx encoding schemes
var BatchV2TxsType = BatchV2TxsV3Type

// TODO: fix hardcoded chainID
var ChainID = big.NewInt(1337)

type BatchV1 struct {
	ParentHash common.Hash  // parent L2 block hash
	EpochNum   rollup.Epoch // aka l1 num
	EpochHash  common.Hash  // block hash
	Timestamp  uint64
	// no feeRecipient address input, all fees go to a L2 contract
	Transactions []hexutil.Bytes
}

type BatchV2Prefix struct {
	RelTimestamp  uint64
	L1OriginNum   uint64
	ParentCheck   []byte
	L1OriginCheck []byte
}

type BatchV2Txs interface {
	Encode(w io.Writer) error
	Decode(r *bytes.Reader) error
	// FullTxs returns list of raw full transactions from BatchV2Txs
	FullTxs() ([][]byte, error)
}

type BatchV2Payload struct {
	BlockCount    uint64
	OriginBits    *big.Int
	BlockTxCounts []uint64

	Txs BatchV2Txs

	FeeRecipents []common.Address
}

type BatchV2Version byte

const (
	BatchV2V1 = iota
	BatchV2V2
)

type BatchV2 struct {
	BatchV2Version
	BatchV2Prefix
	BatchV2Payload
}

// custom implementation of unmarshaling json because Txs field is an interface
func (b *BatchV2) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &b.BatchV2Version); err != nil {
		return err
	}
	if err := json.Unmarshal(data, &b.BatchV2Prefix); err != nil {
		return err
	}
	type BatchV2PayloadWithoutTxs struct {
		BlockCount    uint64
		OriginBits    *big.Int
		BlockTxCounts []uint64
	}
	var payload BatchV2PayloadWithoutTxs
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	b.BlockCount = payload.BlockCount
	b.OriginBits = payload.OriginBits
	b.BlockTxCounts = payload.BlockTxCounts
	// manually fill in Tx field
	totalBlockTxCount := uint64(0)
	for _, blockTxCount := range b.BlockTxCounts {
		totalBlockTxCount += blockTxCount
	}
	// super hacky but do not know the better way
	switch BatchV2TxsType {
	case BatchV2TxsV1Type:
		type BatchV2PayloadTxs struct {
			Txs BatchV2TxsV1
		}
		var txWrapper BatchV2PayloadTxs
		if err := json.Unmarshal(data, &txWrapper); err != nil {
			return err
		}
		txWrapper.Txs.TotalBlockTxCount = totalBlockTxCount
		b.Txs = &txWrapper.Txs
	case BatchV2TxsV2Type:
		type BatchV2PayloadTxs struct {
			Txs BatchV2TxsV2
		}
		var txWrapper BatchV2PayloadTxs
		if err := json.Unmarshal(data, &txWrapper); err != nil {
			return err
		}
		b.Txs = &txWrapper.Txs
	case BatchV2TxsV3Type:
		type BatchV2PayloadTxs struct {
			Txs BatchV2TxsV3
		}
		var txWrapper BatchV2PayloadTxs
		if err := json.Unmarshal(data, &txWrapper); err != nil {
			return err
		}
		txWrapper.Txs.TotalBlockTxCount = totalBlockTxCount
		// TODO: fix hardcoded chainID
		txWrapper.Txs.ChainID = ChainID
		b.Txs = &txWrapper.Txs
	default:
		return fmt.Errorf("invalid BatchV2TxsType: %d", BatchV2TxsV2Type)
	}
	return nil
}

type BatchData struct {
	BatchType int
	BatchV1
	BatchV2
	// batches may contain additional data with new upgrades
}

func InitBatchDataV2(batchV2 BatchV2) *BatchData {
	return &BatchData{
		BatchType: BatchV2Type,
		BatchV2:   batchV2,
	}
}

// DecodeVersion parses BatchV2 version
func (b *BatchV2) DecodeVersion(r *bytes.Reader) error {
	version, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("failed to read version: %w", err)
	}
	if int(version) > BatchV2V2 {
		return fmt.Errorf("invalid version: %d", version)
	}
	b.BatchV2Version = BatchV2Version(version)
	return nil
}

// DecodePrefix parses data into b.BatchV2Prefix
func (b *BatchV2) DecodePrefix(r *bytes.Reader) error {
	relTimestamp, err := binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("failed to read rel timestamp: %w", err)
	}
	b.RelTimestamp = relTimestamp
	L1OriginNum, err := binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("failed to read l1 origin num: %w", err)
	}
	b.L1OriginNum = L1OriginNum
	b.ParentCheck = make([]byte, 20)
	_, err = io.ReadFull(r, b.ParentCheck)
	if err != nil {
		return fmt.Errorf("failed to read parent check: %w", err)
	}
	b.L1OriginCheck = make([]byte, 20)
	_, err = io.ReadFull(r, b.L1OriginCheck)
	if err != nil {
		return fmt.Errorf("failed to read l1 origin check: %w", err)
	}
	return nil
}

func (b *BatchV2Payload) DecodeOriginBits(originBitBuffer []byte, blockCount uint64) {
	originBits := new(big.Int)
	for i := 0; i < int(blockCount); i += 8 {
		end := i + 8
		if end < int(blockCount) {
			end = int(blockCount)
		}
		bits := originBitBuffer[i/8]
		for j := i; j < end; j++ {
			bit := uint((bits >> (j - i)) & 1)
			originBits.SetBit(originBits, j, bit)
		}
	}
	b.OriginBits = originBits
}

func (b *BatchV2) DecodeFeeRecipents(r *bytes.Reader) error {
	if b.BatchV2Version < BatchV2V2 {
		return nil
	}
	var idxs []uint64
	cardinalFeeRecipentsCount := uint64(0)
	for i := 0; i < int(b.BlockCount); i++ {
		idx, err := binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("failed to read fee recipent index: %w", err)
		}
		idxs = append(idxs, idx)
		if cardinalFeeRecipentsCount < idx+1 {
			cardinalFeeRecipentsCount = idx + 1
		}
	}
	var cardinalFeeRecipents []common.Address
	for i := 0; i < int(cardinalFeeRecipentsCount); i++ {
		feeRecipent := make([]byte, common.AddressLength)
		_, err := io.ReadFull(r, feeRecipent)
		if err != nil {
			return fmt.Errorf("failed to read fee recipent address: %w", err)
		}
		cardinalFeeRecipents = append(cardinalFeeRecipents, common.BytesToAddress(feeRecipent))
	}
	b.FeeRecipents = make([]common.Address, 0)
	for _, idx := range idxs {
		feeRecipent := cardinalFeeRecipents[idx]
		b.FeeRecipents = append(b.FeeRecipents, feeRecipent)
	}
	return nil
}

// DecodePayload parses data into b.BatchV2Payload
func (b *BatchV2) DecodePayload(r *bytes.Reader) error {
	blockCount, err := binary.ReadUvarint(r)
	// TODO: check block count is not too large
	if err != nil {
		return fmt.Errorf("failed to read block count: %w", err)
	}
	originBitBufferLen := blockCount / 8
	if blockCount%8 != 0 {
		originBitBufferLen++
	}
	originBitBuffer := make([]byte, originBitBufferLen)
	_, err = io.ReadFull(r, originBitBuffer)
	if err != nil {
		return fmt.Errorf("failed to read origin bits: %w", err)
	}
	blockTxCounts := make([]uint64, blockCount)
	totalBlockTxCount := uint64(0)
	for i := 0; i < int(blockCount); i++ {
		blockTxCount, err := binary.ReadUvarint(r)
		// TODO: check blockTxCount is not too large
		if err != nil {
			return fmt.Errorf("failed to read block tx count: %w", err)
		}
		blockTxCounts[i] = blockTxCount
		totalBlockTxCount += blockTxCount
	}
	// abstract out by BatchV2Txs type
	switch BatchV2TxsType {
	case BatchV2TxsV1Type:
		b.Txs = &BatchV2TxsV1{}
		b.Txs.(*BatchV2TxsV1).TotalBlockTxCount = totalBlockTxCount
	case BatchV2TxsV2Type:
		b.Txs = &BatchV2TxsV2{}
	case BatchV2TxsV3Type:
		b.Txs = &BatchV2TxsV3{}
		b.Txs.(*BatchV2TxsV3).TotalBlockTxCount = totalBlockTxCount
		// TODO: fix hardcoded chainID
		b.Txs.(*BatchV2TxsV3).ChainID = ChainID
	default:
		return fmt.Errorf("invalid BatchV2TxsType: %d", BatchV2TxsV2Type)
	}
	b.Txs.Decode(r)
	b.BlockCount = blockCount
	b.DecodeOriginBits(originBitBuffer, blockCount)
	b.BlockTxCounts = blockTxCounts
	b.DecodeFeeRecipents(r)
	return nil
}

// DecodeBytes parses data into b from data
func (b *BatchV2) DecodeBytes(data []byte) error {
	r := bytes.NewReader(data)
	if err := b.DecodeVersion(r); err != nil {
		return err
	}
	if err := b.DecodePrefix(r); err != nil {
		return err
	}
	if err := b.DecodePayload(r); err != nil {
		return err
	}
	return nil
}

func (b *BatchV2) EncodePrefix(w io.Writer) error {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], b.RelTimestamp)
	if _, err := w.Write(buf[:n]); err != nil {
		return fmt.Errorf("cannot write rel timestamp: %w", err)
	}
	n = binary.PutUvarint(buf[:], b.L1OriginNum)
	if _, err := w.Write(buf[:n]); err != nil {
		return fmt.Errorf("cannot write l1 origin number: %w", err)
	}
	if _, err := w.Write(b.ParentCheck); err != nil {
		return fmt.Errorf("cannot write parent check: %w", err)
	}
	if _, err := w.Write(b.L1OriginCheck); err != nil {
		return fmt.Errorf("cannot write l1 origin check: %w", err)
	}
	return nil
}

func (b BatchV2) PrefixSize() (int, error) {
	buf := encodeBufferPool.Get().(*bytes.Buffer)
	defer encodeBufferPool.Put(buf)
	buf.Reset()
	if err := b.EncodePrefix(buf); err != nil {
		return 0, err
	}
	return buf.Len(), nil
}

func (b BatchV2) MetadataSize() (int, error) {
	// define metadata as every data except tx related field
	// start with 1 because of version field
	size := 1
	prefixSize, err := b.PrefixSize()
	if err != nil {
		return 0, err
	}
	size += prefixSize
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], b.BlockCount)
	size += n
	originBitBuffer := b.EncodeOriginBits()
	size += len(originBitBuffer)
	for _, blockTxCount := range b.BlockTxCounts {
		n = binary.PutUvarint(buf[:], blockTxCount)
		size += n
	}
	return size, nil
}

func (b *BatchV2Payload) EncodeOriginBits() []byte {
	originBitBufferLen := b.BlockCount / 8
	if b.BlockCount%8 != 0 {
		originBitBufferLen++
	}
	originBitBuffer := make([]byte, originBitBufferLen)
	for i := 0; i < int(b.BlockCount); i += 8 {
		end := i + 8
		if end < int(b.BlockCount) {
			end = int(b.BlockCount)
		}
		var bits uint = 0
		for j := i; j < end; j++ {
			bits |= b.OriginBits.Bit(j) << (j - i)
		}
		originBitBuffer[i/8] = byte(bits)
	}
	return originBitBuffer
}

func (b *BatchV2) EncodeVersion(w io.Writer) error {
	if _, err := w.Write([]byte{byte(b.BatchV2Version)}); err != nil {
		return fmt.Errorf("cannot write version: %w", err)
	}
	return nil
}

// EncodeFeeRecipents parses data into b.FeeRecipents
func (b *BatchV2) EncodeFeeRecipents(w io.Writer) error {
	if b.BatchV2Version < BatchV2V2 {
		return nil
	}
	var buf [binary.MaxVarintLen64]byte
	acc := uint64(0)
	indexs := make(map[common.Address]uint64)
	var cardinalFeeRecipents []common.Address
	for _, feeRecipent := range b.FeeRecipents {
		_, exists := indexs[feeRecipent]
		if exists {
			continue
		}
		cardinalFeeRecipents = append(cardinalFeeRecipents, feeRecipent)
		indexs[feeRecipent] = acc
		acc++
	}
	for _, feeReceipt := range b.FeeRecipents {
		idx := indexs[feeReceipt]
		n := binary.PutUvarint(buf[:], idx)
		if _, err := w.Write(buf[:n]); err != nil {
			return fmt.Errorf("cannot write fee recipent index: %w", err)
		}
	}
	for _, cardinalFeeReceipt := range cardinalFeeRecipents {
		if _, err := w.Write(cardinalFeeReceipt[:]); err != nil {
			return fmt.Errorf("cannot write fee recipent address: %w", err)
		}
	}
	return nil
}

func (b *BatchV2) EncodePayload(w io.Writer) error {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], b.BlockCount)
	if _, err := w.Write(buf[:n]); err != nil {
		return fmt.Errorf("cannot write block count: %w", err)
	}
	originBitBuffer := b.EncodeOriginBits()
	if _, err := w.Write(originBitBuffer); err != nil {
		return fmt.Errorf("cannot write origin bits: %w", err)
	}
	for _, blockTxCount := range b.BlockTxCounts {
		n = binary.PutUvarint(buf[:], blockTxCount)
		if _, err := w.Write(buf[:n]); err != nil {
			return fmt.Errorf("cannot write block tx count: %w", err)
		}
	}
	if err := b.Txs.Encode(w); err != nil {
		return err
	}
	if err := b.EncodeFeeRecipents(w); err != nil {
		return err
	}
	return nil
}

// Encode writes the byte encoding of b to w
func (b *BatchV2) Encode(w io.Writer) error {
	if err := b.EncodeVersion(w); err != nil {
		return err
	}
	if err := b.EncodePrefix(w); err != nil {
		return err
	}
	if err := b.EncodePayload(w); err != nil {
		return err
	}
	return nil
}

// EncodeBytes returns the byte encoding of b
func (b *BatchV2) EncodeBytes() ([]byte, error) {
	buf := encodeBufferPool.Get().(*bytes.Buffer)
	defer encodeBufferPool.Put(buf)
	buf.Reset()
	if err := b.Encode(buf); err != nil {
		return []byte{}, err
	}
	return buf.Bytes(), nil
}

func (b *BatchV1) Epoch() eth.BlockID {
	return eth.BlockID{Hash: b.EpochHash, Number: uint64(b.EpochNum)}
}

func InitBatchDataV1(batchV1 BatchV1) *BatchData {
	return &BatchData{
		BatchType: BatchV1Type,
		BatchV1:   batchV1,
	}
}

// EncodeRLP implements rlp.Encoder
func (b *BatchData) EncodeRLP(w io.Writer) error {
	buf := encodeBufferPool.Get().(*bytes.Buffer)
	defer encodeBufferPool.Put(buf)
	buf.Reset()
	if err := b.encodeTyped(buf); err != nil {
		return err
	}
	return rlp.Encode(w, buf.Bytes())
}

// MarshalBinary returns the canonical encoding of the batch.
func (b *BatchData) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	err := b.encodeTyped(&buf)
	return buf.Bytes(), err
}

func (b *BatchData) encodeTyped(buf *bytes.Buffer) error {
	switch b.BatchType {
	case BatchV1Type:
		buf.WriteByte(BatchV1Type)
		return rlp.Encode(buf, &b.BatchV1)
	case BatchV2Type:
		buf.WriteByte(BatchV2Type)
		return b.BatchV2.Encode(buf)
	default:
		return fmt.Errorf("unrecognized batch type: %d", b.BatchType)
	}
}

// DecodeRLP implements rlp.Decoder
func (b *BatchData) DecodeRLP(s *rlp.Stream) error {
	if b == nil {
		return errors.New("cannot decode into nil BatchData")
	}
	v, err := s.Bytes()
	if err != nil {
		return err
	}
	return b.decodeTyped(v)
}

// UnmarshalBinary decodes the canonical encoding of batch.
func (b *BatchData) UnmarshalBinary(data []byte) error {
	if b == nil {
		return errors.New("cannot decode into nil BatchData")
	}
	return b.decodeTyped(data)
}

func (b *BatchData) decodeTyped(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("batch too short")
	}
	switch data[0] {
	case BatchV1Type:
		b.BatchType = BatchV1Type
		return rlp.DecodeBytes(data[1:], &b.BatchV1)
	case BatchV2Type:
		b.BatchType = BatchV2Type
		return b.BatchV2.DecodeBytes(data[1:])
	default:
		return fmt.Errorf("unrecognized batch type: %d", data[0])
	}
}

// MergeBatchV1s merges BatchV1 List and initialize single BatchV2
func (b *BatchV2) MergeBatchV1s(batchV1s []BatchV1, originChangedBit uint, genesisTimestamp uint64) error {
	if len(batchV1s) == 0 {
		return errors.New("cannot merge empty batchV1 list")
	}
	// Sort by timestamp of L2 block
	sort.Slice(batchV1s, func(i, j int) bool {
		return batchV1s[i].Timestamp < batchV1s[j].Timestamp
	})
	// BatchV2Prefix
	span_start := batchV1s[0]
	span_end := batchV1s[len(batchV1s)-1]
	b.RelTimestamp = span_start.Timestamp - genesisTimestamp
	b.L1OriginNum = uint64(span_end.EpochNum)
	b.ParentCheck = make([]byte, 20)
	copy(b.ParentCheck, span_start.ParentHash[:20])
	b.L1OriginCheck = make([]byte, 20)
	copy(b.L1OriginCheck, span_end.EpochHash[:20])
	// BatchV2Payload
	b.BlockCount = uint64(len(batchV1s))
	b.OriginBits = new(big.Int)
	b.OriginBits.SetBit(b.OriginBits, 0, originChangedBit)
	for i := 1; i < len(batchV1s); i++ {
		bit := uint(0)
		if batchV1s[i-1].EpochNum < batchV1s[i].EpochNum {
			bit = 1
		}
		b.OriginBits.SetBit(b.OriginBits, i, bit)
	}

	var blockTxCounts []uint64
	var txs [][]byte
	for _, batchV1 := range batchV1s {
		blockTxCount := uint64(len(batchV1.Transactions))
		blockTxCounts = append(blockTxCounts, blockTxCount)
		for _, rawTx := range batchV1.Transactions {
			txs = append(txs, rawTx)
		}
	}
	b.BlockTxCounts = blockTxCounts
	// abstract out by BatchV2Txs type
	var batchV2Txs BatchV2Txs
	var err error
	switch BatchV2TxsType {
	case BatchV2TxsV1Type:
		batchV2Txs, err = NewBatchV2TxsV1(txs)
	case BatchV2TxsV2Type:
		batchV2Txs, err = NewBatchV2TxsV2(txs)
	case BatchV2TxsV3Type:
		batchV2Txs, err = NewBatchV2TxsV3(txs)
	default:
		return fmt.Errorf("invalid BatchV2TxsType: %d", BatchV2TxsType)
	}
	if err != nil {
		return err
	}
	b.Txs = batchV2Txs
	return nil
}

// SplitBatchV2 splits single BatchV2 and initialize BatchV1 lists
// Cannot fill every BatchV1 parent hash
func (b *BatchV2) SplitBatchV2(fetchL1Block func(uint64) (*types.Block, error), blockTime, genesisTimestamp uint64) ([]BatchV1, error) {
	batchV1s := make([]BatchV1, b.BlockCount)
	for i := 0; i < int(b.BlockCount); i++ {
		batchV1s[i].Timestamp = b.RelTimestamp + genesisTimestamp + uint64(i)*blockTime
	}
	// first fetch last L2 block in BatchV2
	var l1OriginBlock *types.Block
	var l1OriginBlockNumber = b.L1OriginNum
	var err error
	l1OriginBlock, err = fetchL1Block(l1OriginBlockNumber)
	if err != nil {
		return nil, err
	}
	// backward digest leftover L2 blocks
	for i := int(b.BlockCount) - 1; i >= 0; i-- {
		batchV1s[i].EpochNum = rollup.Epoch(l1OriginBlockNumber)
		batchV1s[i].EpochHash = l1OriginBlock.Hash()
		// no need to fetch L1 when first block because not used
		if b.OriginBits.Bit(i) == 1 && i > 0 {
			l1OriginBlockNumber--
			l1OriginBlock, err = fetchL1Block(l1OriginBlockNumber)
			if err != nil {
				return nil, err
			}
		}
	}

	txs, err := b.Txs.FullTxs()
	if err != nil {
		return nil, err
	}
	idx := 0
	for i := 0; i < int(b.BlockCount); i++ {
		for txIndex := 0; txIndex < int(b.BlockTxCounts[i]); txIndex++ {
			batchV1s[i].Transactions = append(batchV1s[i].Transactions, txs[idx])
			idx++
		}
	}
	return batchV1s, nil
}

// SplitBatchV2CheckValidation splits single BatchV2 and initialize BatchV1 lists and validates ParentCheck and L1OriginCheck
// Cannot fill in BatchV1 parent hash except the first BatchV1 because we cannot trust other L2s yet.
func (b *BatchV2) SplitBatchV2CheckValidation(fetchL1Block func(uint64) (*types.Block, error), safeL2Head eth.L2BlockRef, blockTime, genesisTimestamp uint64) ([]BatchV1, error) {
	batchV1s, err := b.SplitBatchV2(fetchL1Block, blockTime, genesisTimestamp)
	if err != nil {
		return nil, err
	}
	// set only the first batchV1's parent hash
	batchV1s[0].ParentHash = safeL2Head.Hash
	if !bytes.Equal(safeL2Head.Hash.Bytes()[:20], b.ParentCheck) {
		return nil, errors.New("parent hash mismatch")
	}
	l1OriginBlockHash := batchV1s[len(batchV1s)-1].EpochHash
	if !bytes.Equal(l1OriginBlockHash[:20], b.L1OriginCheck) {
		return nil, errors.New("l1 origin hash mismatch")
	}
	return batchV1s, nil
}

package derive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
	"io"
	"math/big"
	"sort"
)

type SpanBatchPrefix struct {
	RelTimestamp  uint64
	L1OriginNum   uint64
	ParentCheck   []byte
	L1OriginCheck []byte
}

type SpanBatchSignature struct {
	V uint64
	R *uint256.Int
	S *uint256.Int
}

type SpanBatchPayload struct {
	BlockCount    uint64
	OriginBits    *big.Int
	BlockTxCounts []uint64
	Txs           *SpanBatchTxs
	FeeRecipents  []common.Address
}

type SpanBatchDerivedFields struct {
	IsDerived       bool
	BatchTimestamp  uint64
	BlockTimestamps []uint64
	BlockOriginNums []uint64
}

type SpanBatch struct {
	SpanBatchPrefix
	SpanBatchPayload
	SpanBatchDerivedFields
}

func (b *SpanBatch) GetBatchType() int {
	return SpanBatchType
}

func (b *SpanBatch) GetTimestamp() uint64 {
	if !b.IsDerived {
		panic("Span batch fields are not derived yet")
	}
	return b.BatchTimestamp
}

func (b *SpanBatch) GetEpochNum() rollup.Epoch {
	if !b.IsDerived {
		panic("Span batch fields are not derived yet")
	}
	return rollup.Epoch(b.BlockOriginNums[0])
}

func (b *SpanBatch) GetLogContext(log log.Logger) log.Logger {
	return log.New(
		"batch_timestamp", b.BatchTimestamp,
		"parent_check", b.ParentCheck,
		"origin_check", b.L1OriginCheck,
		"origin_number", b.L1OriginNum,
		"epoch_number", b.GetEpochNum(),
		"blocks", b.BlockCount,
	)
}

func (b *SpanBatch) CheckOriginHash(hash common.Hash) bool {
	return bytes.Equal(b.L1OriginCheck, hash.Bytes()[:20])
}

func (b *SpanBatch) CheckParentHash(hash common.Hash) bool {
	return bytes.Equal(b.ParentCheck, hash.Bytes()[:20])
}

func (b *SpanBatchPayload) DecodeOriginBits(originBitBuffer []byte, blockCount uint64) {
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

// DecodePrefix parses data into b.SpanBatchPrefix
func (b *SpanBatch) DecodePrefix(r *bytes.Reader) error {
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

// DecodePayload parses data into b.SpanBatchPayload
func (b *SpanBatch) DecodePayload(r *bytes.Reader) error {
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
	b.DecodeOriginBits(originBitBuffer, blockCount)
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
	b.BlockCount = blockCount
	b.BlockTxCounts = blockTxCounts
	b.Txs = &SpanBatchTxs{}
	b.Txs.TotalBlockTxCount = totalBlockTxCount
	if err = b.Txs.Decode(r); err != nil {
		return err
	}
	return nil
}

// DecodeBytes parses data into b from data
func (b *SpanBatch) DecodeBytes(data []byte) error {
	r := bytes.NewReader(data)
	if err := b.DecodePrefix(r); err != nil {
		return err
	}
	if err := b.DecodePayload(r); err != nil {
		return err
	}
	return nil
}

func (b *SpanBatch) EncodePrefix(w io.Writer) error {
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

func (b *SpanBatchPayload) EncodeOriginBits() []byte {
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

func (b *SpanBatch) EncodePayload(w io.Writer) error {
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
	return nil
}

// Encode writes the byte encoding of b to w
func (b *SpanBatch) Encode(w io.Writer) error {
	if err := b.EncodePrefix(w); err != nil {
		return err
	}
	if err := b.EncodePayload(w); err != nil {
		return err
	}
	return nil
}

// EncodeBytes returns the byte encoding of b
func (b *SpanBatch) EncodeBytes() ([]byte, error) {
	buf := encodeBufferPool.Get().(*bytes.Buffer)
	defer encodeBufferPool.Put(buf)
	buf.Reset()
	if err := b.Encode(buf); err != nil {
		return []byte{}, err
	}
	return buf.Bytes(), nil
}

// MergeSingularBatches merges SingularBatch List and initialize single SpanBatch
func (b *SpanBatch) MergeSingularBatches(singularBatches []*SingularBatch, originChangedBit uint, genesisTimestamp uint64, chainId *big.Int) error {
	if len(singularBatches) == 0 {
		return errors.New("cannot merge empty singularBatch list")
	}
	// Sort by timestamp of L2 block
	sort.Slice(singularBatches, func(i, j int) bool {
		return singularBatches[i].Timestamp < singularBatches[j].Timestamp
	})
	// SpanBatchPrefix
	span_start := singularBatches[0]
	span_end := singularBatches[len(singularBatches)-1]
	b.RelTimestamp = span_start.Timestamp - genesisTimestamp
	b.L1OriginNum = uint64(span_end.EpochNum)
	b.ParentCheck = make([]byte, 20)
	copy(b.ParentCheck, span_start.ParentHash[:20])
	b.L1OriginCheck = make([]byte, 20)
	copy(b.L1OriginCheck, span_end.EpochHash[:20])
	// SpanBatchPayload
	b.BlockCount = uint64(len(singularBatches))
	b.OriginBits = new(big.Int)
	b.OriginBits.SetBit(b.OriginBits, 0, originChangedBit)
	for i := 1; i < len(singularBatches); i++ {
		bit := uint(0)
		if singularBatches[i-1].EpochNum < singularBatches[i].EpochNum {
			bit = 1
		}
		b.OriginBits.SetBit(b.OriginBits, i, bit)
	}
	var blockTxCounts []uint64
	var txs [][]byte
	var blockTimstamps []uint64
	var blockOriginNums []uint64
	for _, singularBatch := range singularBatches {
		blockTxCount := uint64(len(singularBatch.Transactions))
		blockTxCounts = append(blockTxCounts, blockTxCount)
		blockTimstamps = append(blockTimstamps, singularBatch.Timestamp)
		blockOriginNums = append(blockOriginNums, uint64(singularBatch.EpochNum))
		for _, rawTx := range singularBatch.Transactions {
			txs = append(txs, rawTx)
		}
	}
	b.BlockTxCounts = blockTxCounts
	spanBatchTxs, err := NewSpanBatchTxs(txs, chainId)
	if err != nil {
		return err
	}
	b.Txs = spanBatchTxs
	b.BatchTimestamp = blockTimstamps[0]
	b.BlockTimestamps = blockTimstamps
	b.BlockOriginNums = blockOriginNums
	b.IsDerived = true
	return nil
}

// SplitSpanBatch splits single SpanBatch and initialize SingularBatch lists
// Cannot fill every SingularBatch parent hash
func (b *SpanBatch) SplitSpanBatch(l1Origins []eth.L1BlockRef) ([]*SingularBatch, error) {
	singularBatches := make([]*SingularBatch, b.BlockCount)
	originIdx := -1
	for i := 0; i < len(l1Origins); i++ {
		if l1Origins[i].Number == b.BlockOriginNums[0] {
			originIdx = i
			break
		}
	}
	if originIdx == -1 {
		return nil, fmt.Errorf("cannot find L1 origin")
	}
	txs, err := b.Txs.FullTxs()
	if err != nil {
		return nil, err
	}
	txIdx := 0
	for i := 0; i < int(b.BlockCount); i++ {
		singularBatch := SingularBatch{}
		singularBatch.Timestamp = b.BlockTimestamps[i]
		singularBatch.EpochNum = rollup.Epoch(b.BlockOriginNums[i])
		if b.OriginBits.Bit(i) == 1 && i > 0 {
			originIdx += 1
		}
		singularBatch.EpochHash = l1Origins[originIdx].Hash

		for j := 0; j < int(b.BlockTxCounts[i]); j++ {
			singularBatch.Transactions = append(singularBatch.Transactions, txs[txIdx])
			txIdx++
		}
		singularBatches[i] = &singularBatch
	}
	return singularBatches, nil
}

// SplitSpanBatchCheckValidation splits single SpanBatch and initialize SingularBatch lists and validates ParentCheck and L1OriginCheck
// Cannot fill in SingularBatch parent hash except the first SingularBatch because we cannot trust other L2s yet.
func (b *SpanBatch) SplitSpanBatchCheckValidation(l1Origins []eth.L1BlockRef, safeL2Head eth.L2BlockRef) ([]*SingularBatch, error) {
	singularBatches, err := b.SplitSpanBatch(l1Origins)
	if err != nil {
		return nil, err
	}
	// set only the first singularBatch's parent hash
	singularBatches[0].ParentHash = safeL2Head.Hash
	if !bytes.Equal(safeL2Head.Hash.Bytes()[:20], b.ParentCheck) {
		return nil, errors.New("parent hash mismatch")
	}
	l1OriginBlockHash := singularBatches[len(singularBatches)-1].EpochHash
	if !bytes.Equal(l1OriginBlockHash[:20], b.L1OriginCheck) {
		return nil, errors.New("l1 origin hash mismatch")
	}
	return singularBatches, nil
}

func (b *SpanBatch) DeriveSpanBatchFields(blockTime, genesisTimestamp uint64, chainId *big.Int) {
	b.BatchTimestamp = b.RelTimestamp + genesisTimestamp
	b.BlockTimestamps = make([]uint64, b.BlockCount)
	b.BlockOriginNums = make([]uint64, b.BlockCount)

	var l1OriginBlockNumber = b.L1OriginNum
	for i := int(b.BlockCount) - 1; i >= 0; i-- {
		b.BlockTimestamps[i] = b.BatchTimestamp + uint64(i)*blockTime
		b.BlockOriginNums[i] = l1OriginBlockNumber
		if b.OriginBits.Bit(i) == 1 && i > 0 {
			l1OriginBlockNumber--
		}
	}

	b.Txs.ChainID = chainId
	b.Txs.RecoverV()
	b.IsDerived = true
}

func (b *SpanBatch) AppendSingularBatch(singularBatch *SingularBatch) error {
	b.BlockCount += 1
	originBit := uint(0)
	if b.L1OriginNum != uint64(singularBatch.EpochNum) {
		originBit = 1
	}
	b.OriginBits.SetBit(b.OriginBits, int(b.BlockCount-1), originBit)
	b.L1OriginNum = uint64(singularBatch.EpochNum)
	b.L1OriginCheck = singularBatch.EpochHash.Bytes()[:20]
	b.BlockTxCounts = append(b.BlockTxCounts, uint64(len(singularBatch.Transactions)))
	for _, rawTx := range singularBatch.Transactions {
		if err := b.Txs.AppendTx(rawTx); err != nil {
			return err
		}
	}
	b.BlockTimestamps = append(b.BlockTimestamps, singularBatch.Timestamp)
	b.BlockOriginNums = append(b.BlockOriginNums, uint64(singularBatch.EpochNum))
	return nil
}

type SpanBatchBuilder struct {
	parentEpochHash  common.Hash
	genesisTimestamp uint64
	chainId          *big.Int
	spanBatch        *SpanBatch
}

func NewSpanBatchBuilder(parentEpochHash common.Hash, genesisTimestamp uint64, chainId *big.Int) *SpanBatchBuilder {
	return &SpanBatchBuilder{
		parentEpochHash:  parentEpochHash,
		genesisTimestamp: genesisTimestamp,
		chainId:          chainId,
		spanBatch:        &SpanBatch{},
	}
}

func (b *SpanBatchBuilder) AppendSingularBatch(singularBatch *SingularBatch) error {
	if b.spanBatch.BlockCount == 0 {
		originChangedBit := 0
		if singularBatch.EpochHash != b.parentEpochHash {
			originChangedBit = 1
		}
		return b.spanBatch.MergeSingularBatches([]*SingularBatch{singularBatch}, uint(originChangedBit), b.genesisTimestamp, b.chainId)
	}
	return b.spanBatch.AppendSingularBatch(singularBatch)
}

func (b *SpanBatchBuilder) GetSpanBatch() *SpanBatch {
	return b.spanBatch
}

func (b *SpanBatchBuilder) GetBlockCount() uint64 {
	return b.spanBatch.BlockCount
}

func (b *SpanBatchBuilder) Reset() {
	b.spanBatch = &SpanBatch{}
}

// ReadTxData reads raw RLP tx data from reader
func ReadTxData(r *bytes.Reader) ([]byte, int, error) {
	var txData []byte
	offset, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to seek tx reader: %w", err)
	}
	b, err := r.ReadByte()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read tx initial byte: %w", err)
	}
	txType := byte(0)
	if int(b) <= 0x7F {
		// EIP-2718: non legacy tx so write tx type
		txType = byte(b)
		txData = append(txData, txType)
	} else {
		// legacy tx: seek back single byte to read prefix again
		_, err = r.Seek(offset, io.SeekStart)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to seek tx reader: %w", err)
		}
	}
	// TODO: set maximum inputLimit
	s := rlp.NewStream(r, 0)
	var txPayload []byte
	kind, _, err := s.Kind()
	switch {
	case err != nil:
		return nil, 0, fmt.Errorf("failed to read tx RLP prefix: %w", err)
	case kind == rlp.List:
		if txPayload, err = s.Raw(); err != nil {
			return nil, 0, fmt.Errorf("failed to read tx RLP payload: %w", err)
		}
	default:
		return nil, 0, errors.New("tx RLP prefix type must be list")
	}
	txData = append(txData, txPayload...)
	return txData, int(txType), nil
}

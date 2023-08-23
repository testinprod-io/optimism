package derive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
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
	"github.com/holiman/uint256"
)

// Batch format
// first byte is type followed by bytestring.
//
// SingularBatchType := 0
// singularBatch := SingularBatchType ++ RLP([epoch, timestamp, transaction_list]
//
// An empty input is not a valid batch.
//
// Note: the type system is based on L1 typed transactions.
// SpanBatchType := 1
// spanBatch := SpanBatchType ++ prefix ++ payload
// prefix := rel_timestamp ++ l1_origin_num ++ parent_check ++ l1_origin_check
// payload := block_count ++ origin_bits ++ block_tx_counts ++ tx_data ++ tx_sigs

// encodeBufferPool holds temporary encoder buffers for batch encoding
var encodeBufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

const (
	SingularBatchType = iota
	SpanBatchType
)

type Batch interface {
	GetBatchType() int
	GetTimestamp() uint64
	GetEpochNum() rollup.Epoch
	GetLogContext(log.Logger) log.Logger
	CheckOriginHash(common.Hash) bool
	CheckParentHash(common.Hash) bool
}

type SingularBatch struct {
	ParentHash common.Hash  // parent L2 block hash
	EpochNum   rollup.Epoch // aka l1 num
	EpochHash  common.Hash  // block hash
	Timestamp  uint64
	// no feeRecipient address input, all fees go to a L2 contract
	Transactions []hexutil.Bytes
}

func (b *SingularBatch) GetBatchType() int {
	return SingularBatchType
}

func (b *SingularBatch) GetTimestamp() uint64 {
	return b.Timestamp
}

func (b *SingularBatch) GetEpochNum() rollup.Epoch {
	return b.EpochNum
}

func (b *SingularBatch) GetLogContext(log log.Logger) log.Logger {
	return log.New(
		"batch_timestamp", b.Timestamp,
		"parent_hash", b.ParentHash,
		"batch_epoch", b.Epoch(),
		"txs", len(b.Transactions),
	)
}

func (b *SingularBatch) CheckOriginHash(hash common.Hash) bool {
	return b.EpochHash == hash
}

func (b *SingularBatch) CheckParentHash(hash common.Hash) bool {
	return b.ParentHash == hash
}

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

	// below fields may be generalized
	TxDatas []hexutil.Bytes
	TxSigs  []SpanBatchSignature
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

type BatchData struct {
	BatchType int
	SingularBatch
	SpanBatch
	// batches may contain additional data with new upgrades
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
	case SingularBatchType:
		buf.WriteByte(SingularBatchType)
		return rlp.Encode(buf, &b.SingularBatch)
	case SpanBatchType:
		buf.WriteByte(SpanBatchType)
		return b.SpanBatch.Encode(buf)
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
	case SingularBatchType:
		b.BatchType = SingularBatchType
		return rlp.DecodeBytes(data[1:], &b.SingularBatch)
	case SpanBatchType:
		b.BatchType = SpanBatchType
		return b.SpanBatch.DecodeBytes(data[1:])
	default:
		return fmt.Errorf("unrecognized batch type: %d", data[0])
	}
}

func (b *BatchData) GetBatch() (Batch, error) {
	switch b.BatchType {
	case SingularBatchType:
		return &b.SingularBatch, nil
	case SpanBatchType:
		return &b.SpanBatch, nil
	default:
		return nil, fmt.Errorf("unrecognized batch type: %d", b.BatchType)
	}
}

func NewSingularBatchData(singularBatch SingularBatch) *BatchData {
	return &BatchData{
		BatchType:     SingularBatchType,
		SingularBatch: singularBatch,
	}
}

func NewSpanBatchData(spanBatch SpanBatch) *BatchData {
	return &BatchData{
		BatchType: SpanBatchType,
		SpanBatch: spanBatch,
	}
}

func (b *SingularBatch) Epoch() eth.BlockID {
	return eth.BlockID{Hash: b.EpochHash, Number: uint64(b.EpochNum)}
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

// ReadTxData reads raw RLP tx data from reader
func ReadTxData(r *bytes.Reader) ([]byte, error) {
	var txData []byte
	offset, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, fmt.Errorf("failed to seek tx reader: %w", err)
	}
	b, err := r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("failed to read tx initial byte: %w", err)
	}
	if int(b) <= 0x7F {
		// EIP-2718: non legacy tx so write tx type
		txType := byte(b)
		txData = append(txData, txType)
	} else {
		// legacy tx: seek back single byte to read prefix again
		_, err = r.Seek(offset, io.SeekStart)
		if err != nil {
			return nil, fmt.Errorf("failed to seek tx reader: %w", err)
		}
	}
	// TODO: set maximum inputLimit
	s := rlp.NewStream(r, 0)
	var txPayload []byte
	kind, _, err := s.Kind()
	switch {
	case err != nil:
		return nil, fmt.Errorf("failed to read tx RLP prefix: %w", err)
	case kind == rlp.List:
		if txPayload, err = s.Raw(); err != nil {
			return nil, fmt.Errorf("failed to read tx RLP payload: %w", err)
		}
	default:
		return nil, errors.New("tx RLP prefix type must be list")
	}
	txData = append(txData, txPayload...)
	return txData, nil
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
	// Do not need txDataHeader because RLP byte stream already includes length info
	txDatas := make([]hexutil.Bytes, totalBlockTxCount)
	for i := 0; i < int(totalBlockTxCount); i++ {
		txData, err := ReadTxData(r)
		if err != nil {
			return err
		}
		txDatas[i] = txData
	}
	txSigs := make([]SpanBatchSignature, totalBlockTxCount)
	var sigBuffer [32]byte
	for i := 0; i < int(totalBlockTxCount); i++ {
		var txSig SpanBatchSignature
		v, err := binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("failed to read tx sig v: %w", err)
		}
		txSig.V = v
		_, err = io.ReadFull(r, sigBuffer[:])
		if err != nil {
			return fmt.Errorf("failed to read tx sig r: %w", err)
		}
		txSig.R, _ = uint256.FromBig(new(big.Int).SetBytes(sigBuffer[:]))
		_, err = io.ReadFull(r, sigBuffer[:])
		if err != nil {
			return fmt.Errorf("failed to read tx sig s: %w", err)
		}
		txSig.S, _ = uint256.FromBig(new(big.Int).SetBytes(sigBuffer[:]))
		txSigs[i] = txSig
	}
	b.BlockCount = blockCount
	b.DecodeOriginBits(originBitBuffer, blockCount)
	b.BlockTxCounts = blockTxCounts
	b.TxDatas = txDatas
	b.TxSigs = txSigs
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
	for _, txData := range b.TxDatas {
		if _, err := w.Write(txData); err != nil {
			return fmt.Errorf("cannot write block tx data: %w", err)
		}
	}
	for _, txSig := range b.TxSigs {
		n = binary.PutUvarint(buf[:], txSig.V)
		if _, err := w.Write(buf[:n]); err != nil {
			return fmt.Errorf("cannot write tx sig v: %w", err)
		}
		rBuf := txSig.R.Bytes32()
		if _, err := w.Write(rBuf[:]); err != nil {
			return fmt.Errorf("cannot write tx sig r: %w", err)
		}
		sBuf := txSig.S.Bytes32()
		if _, err := w.Write(sBuf[:]); err != nil {
			return fmt.Errorf("cannot write tx sig s: %w", err)
		}
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
func (b *SpanBatch) MergeSingularBatches(singularBatches []*SingularBatch, originChangedBit uint, genesisTimestamp uint64) error {
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
	var txDatas []hexutil.Bytes
	var txSigs []SpanBatchSignature
	var blockTimstamps []uint64
	var blockOriginNums []uint64
	for _, singularBatch := range singularBatches {
		blockTxCount := uint64(len(singularBatch.Transactions))
		blockTxCounts = append(blockTxCounts, blockTxCount)
		blockTimstamps = append(blockTimstamps, singularBatch.Timestamp)
		blockOriginNums = append(blockOriginNums, uint64(singularBatch.EpochNum))
		for _, rawTx := range singularBatch.Transactions {
			// below segment may be generalized
			var tx types.Transaction
			if err := tx.UnmarshalBinary(rawTx); err != nil {
				return errors.New("failed to decode tx")
			}
			var txSig SpanBatchSignature
			v, r, s := tx.RawSignatureValues()
			R, _ := uint256.FromBig(r)
			S, _ := uint256.FromBig(s)
			txSig.V = v.Uint64()
			txSig.R = R
			txSig.S = S
			txSigs = append(txSigs, txSig)
			spanBatchTx, err := NewSpanBatchTx(tx)
			if err != nil {
				return nil
			}
			txData, err := spanBatchTx.MarshalBinary()
			if err != nil {
				return nil
			}
			txDatas = append(txDatas, txData)
		}
	}
	b.BlockTxCounts = blockTxCounts
	b.TxDatas = txDatas
	b.TxSigs = txSigs
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
	txIdx := 0
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
	for i := 0; i < int(b.BlockCount); i++ {
		singularBatch := SingularBatch{}
		singularBatch.Timestamp = b.BlockTimestamps[i]
		singularBatch.EpochNum = rollup.Epoch(b.BlockOriginNums[i])
		if b.OriginBits.Bit(i) == 1 && i > 0 {
			originIdx += 1
		}
		singularBatch.EpochHash = l1Origins[originIdx].Hash

		for txIndex := 0; txIndex < int(b.BlockTxCounts[i]); txIndex++ {
			var spanBatchTx SpanBatchTx
			if err := spanBatchTx.UnmarshalBinary(b.TxDatas[txIdx]); err != nil {
				return nil, err
			}
			v := new(big.Int).SetUint64(b.TxSigs[txIdx].V)
			r := b.TxSigs[txIdx].R.ToBig()
			s := b.TxSigs[txIdx].S.ToBig()
			tx, err := spanBatchTx.ConvertToFullTx(v, r, s)
			if err != nil {
				return nil, err
			}
			encodedTx, err := tx.MarshalBinary()
			if err != nil {
				return nil, err
			}
			singularBatch.Transactions = append(singularBatch.Transactions, encodedTx)
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

func (b *SpanBatch) DeriveSpanBatchFields(blockTime, genesisTimestamp uint64) {
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
		// below segment may be generalized
		var tx types.Transaction
		if err := tx.UnmarshalBinary(rawTx); err != nil {
			return errors.New("failed to decode tx")
		}
		var txSig SpanBatchSignature
		v, r, s := tx.RawSignatureValues()
		R, _ := uint256.FromBig(r)
		S, _ := uint256.FromBig(s)
		txSig.V = v.Uint64()
		txSig.R = R
		txSig.S = S
		b.TxSigs = append(b.TxSigs, txSig)
		spanBatchTx, err := NewSpanBatchTx(tx)
		if err != nil {
			return err
		}
		txData, err := spanBatchTx.MarshalBinary()
		if err != nil {
			return err
		}
		b.TxDatas = append(b.TxDatas, txData)
	}
	b.BlockTimestamps = append(b.BlockTimestamps, singularBatch.Timestamp)
	b.BlockOriginNums = append(b.BlockOriginNums, uint64(singularBatch.EpochNum))
	return nil
}

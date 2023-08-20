package derive

import (
	"bytes"
	"encoding/binary"
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
	"github.com/holiman/uint256"
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
// payload := block_count ++ origin_bits ++ block_tx_counts ++ tx_data ++ tx_sigs

// encodeBufferPool holds temporary encoder buffers for batch encoding
var encodeBufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

const (
	BatchV1Type = iota
	BatchV2Type
)

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

type BatchV2Signature struct {
	V uint64
	R *uint256.Int
	S *uint256.Int
}

type BatchV2Payload struct {
	BlockCount    uint64
	OriginBits    *big.Int
	BlockTxCounts []uint64

	// below fields may be generalized
	TxDatas []hexutil.Bytes
	TxSigs  []BatchV2Signature
}

type BatchV2Derived struct {
	BatchTimestamp  uint64
	BlockTimestamps []uint64
	BlockOriginNums []uint64
}

type BatchV2 struct {
	BatchV2Prefix
	BatchV2Payload
	BatchV2Derived
}

type BatchData struct {
	BatchType int
	BatchV1
	BatchV2
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

func InitBatchDataV1(batchV1 BatchV1) *BatchData {
	return &BatchData{
		BatchType: BatchV1Type,
		BatchV1:   batchV1,
	}
}

func InitBatchDataV2(batchV2 BatchV2) *BatchData {
	return &BatchData{
		BatchType: BatchV2Type,
		BatchV2:   batchV2,
	}
}

func (b *BatchV1) Epoch() eth.BlockID {
	return eth.BlockID{Hash: b.EpochHash, Number: uint64(b.EpochNum)}
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
	// Do not need txDataHeader because RLP byte stream already includes length info
	txDatas := make([]hexutil.Bytes, totalBlockTxCount)
	for i := 0; i < int(totalBlockTxCount); i++ {
		txData, err := ReadTxData(r)
		if err != nil {
			return err
		}
		txDatas[i] = txData
	}
	txSigs := make([]BatchV2Signature, totalBlockTxCount)
	var sigBuffer [32]byte
	for i := 0; i < int(totalBlockTxCount); i++ {
		var txSig BatchV2Signature
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
func (b *BatchV2) DecodeBytes(data []byte) error {
	r := bytes.NewReader(data)
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
func (b *BatchV2) Encode(w io.Writer) error {
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

// MergeBatchV1s merges BatchV1 List and initialize single BatchV2
func (b *BatchV2) MergeBatchV1s(batchV1s []*BatchV1, originChangedBit uint, genesisTimestamp uint64) error {
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
	var txDatas []hexutil.Bytes
	var txSigs []BatchV2Signature
	var blockTimstamps []uint64
	var blockOriginNums []uint64
	for _, batchV1 := range batchV1s {
		blockTxCount := uint64(len(batchV1.Transactions))
		blockTxCounts = append(blockTxCounts, blockTxCount)
		blockTimstamps = append(blockTimstamps, batchV1.Timestamp)
		blockOriginNums = append(blockOriginNums, uint64(batchV1.EpochNum))
		for _, rawTx := range batchV1.Transactions {
			// below segment may be generalized
			var tx types.Transaction
			if err := tx.UnmarshalBinary(rawTx); err != nil {
				return errors.New("failed to decode tx")
			}
			var txSig BatchV2Signature
			v, r, s := tx.RawSignatureValues()
			R, _ := uint256.FromBig(r)
			S, _ := uint256.FromBig(s)
			txSig.V = v.Uint64()
			txSig.R = R
			txSig.S = S
			txSigs = append(txSigs, txSig)
			batchV2Tx, err := NewBatchV2Tx(tx)
			if err != nil {
				return nil
			}
			txData, err := batchV2Tx.MarshalBinary()
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
	return nil
}

// SplitBatchV2 splits single BatchV2 and initialize BatchV1 lists
// Cannot fill every BatchV1 parent hash
func (b *BatchV2) SplitBatchV2(l1Origins []eth.L1BlockRef) ([]*BatchV1, error) {
	batchV1s := make([]*BatchV1, b.BlockCount)
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
		batchV1 := BatchV1{}
		batchV1.Timestamp = b.BlockTimestamps[i]
		batchV1.EpochNum = rollup.Epoch(b.BlockOriginNums[i])
		if b.OriginBits.Bit(i) == 1 && i > 0 {
			originIdx += 1
		}
		batchV1.EpochHash = l1Origins[originIdx].Hash

		for txIndex := 0; txIndex < int(b.BlockTxCounts[i]); txIndex++ {
			var batchV2Tx BatchV2Tx
			if err := batchV2Tx.UnmarshalBinary(b.TxDatas[txIdx]); err != nil {
				return nil, err
			}
			v := new(big.Int).SetUint64(b.TxSigs[txIdx].V)
			r := b.TxSigs[txIdx].R.ToBig()
			s := b.TxSigs[txIdx].S.ToBig()
			tx, err := batchV2Tx.ConvertToFullTx(v, r, s)
			if err != nil {
				return nil, err
			}
			encodedTx, err := tx.MarshalBinary()
			if err != nil {
				return nil, err
			}
			batchV1.Transactions = append(batchV1.Transactions, encodedTx)
			txIdx++
		}
		batchV1s[i] = &batchV1
	}
	return batchV1s, nil
}

// SplitBatchV2CheckValidation splits single BatchV2 and initialize BatchV1 lists and validates ParentCheck and L1OriginCheck
// Cannot fill in BatchV1 parent hash except the first BatchV1 because we cannot trust other L2s yet.
func (b *BatchV2) SplitBatchV2CheckValidation(l1Origins []eth.L1BlockRef, safeL2Head eth.L2BlockRef) ([]*BatchV1, error) {
	batchV1s, err := b.SplitBatchV2(l1Origins)
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

func (b *BatchV2) DeriveBatchV2Fields(blockTime, genesisTimestamp uint64) {
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
}

func (b *BatchV2) AppendBatchV1(batchV1 *BatchV1) error {
	b.BlockCount += 1
	originBit := uint(0)
	if b.L1OriginNum != uint64(batchV1.EpochNum) {
		originBit = 1
	}
	b.OriginBits.SetBit(b.OriginBits, int(b.BlockCount-1), originBit)
	b.L1OriginNum = uint64(batchV1.EpochNum)
	b.L1OriginCheck = batchV1.EpochHash.Bytes()[:20]
	b.BlockTxCounts = append(b.BlockTxCounts, uint64(len(batchV1.Transactions)))
	for _, rawTx := range batchV1.Transactions {
		// below segment may be generalized
		var tx types.Transaction
		if err := tx.UnmarshalBinary(rawTx); err != nil {
			return errors.New("failed to decode tx")
		}
		var txSig BatchV2Signature
		v, r, s := tx.RawSignatureValues()
		R, _ := uint256.FromBig(r)
		S, _ := uint256.FromBig(s)
		txSig.V = v.Uint64()
		txSig.R = R
		txSig.S = S
		b.TxSigs = append(b.TxSigs, txSig)
		batchV2Tx, err := NewBatchV2Tx(tx)
		if err != nil {
			return err
		}
		txData, err := batchV2Tx.MarshalBinary()
		if err != nil {
			return err
		}
		b.TxDatas = append(b.TxDatas, txData)
	}
	b.BlockTimestamps = append(b.BlockTimestamps, batchV1.Timestamp)
	b.BlockOriginNums = append(b.BlockOriginNums, uint64(batchV1.EpochNum))
	return nil
}

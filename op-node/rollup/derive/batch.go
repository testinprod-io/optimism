package derive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"

	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
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
// prefix := rel_timestamp ++ parent_check ++ l1_origin_check
// payload := block_count ++ origin_bits ++ block_tx_counts ++ tx_data_headers ++ tx_data ++ tx_sigs
//
// len(prefix) = 8 bytes(rel_timestamp) + 20 bytes(parent_check) + 20 bytes(l1_origin_check)

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

const BatchV2PrefixLen = 8 + 20 + 20

type BatchV2Prefix struct {
	Timestamp     uint64
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
	TxDataHeaders []uint64
	TxDatas       []hexutil.Bytes
	TxSigs        []BatchV2Signature
}

type BatchV2 struct {
	BatchV2Prefix
	BatchV2Payload
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

// DecodePrefix parses data into b.BatchV2Prefix from data
func (b *BatchV2) DecodePrefix(data []byte) error {
	if len(data) != BatchV2PrefixLen {
		return fmt.Errorf("invalid prefix length: %d", len(data))
	}
	offset := uint32(0)
	b.Timestamp = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8
	copy(b.ParentCheck, data[offset:offset+20])
	offset += 20
	copy(b.L1OriginCheck, data[offset:offset+20])
	return nil
}

func (b *BatchV2) SetOriginBits(originBitBuffer []byte, blockCount uint64) {
	originBits := new(big.Int)
	numSet := uint64(0)
	for j, bits := range originBitBuffer {
		for i := 0; i < 8; i++ {
			bit := uint(bits & 1)
			originBits = originBits.SetBit(originBits, i+8*j, bit)
			numSet++
			if numSet == blockCount {
				return
			}
			bit >>= 1
		}
	}
}

// DecodePayload parses data into b.BatchV2Payload from data
func (b *BatchV2) DecodePayload(data []byte) error {
	r := bytes.NewReader(data)
	blockCount, err := binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("failed to read block count var int: %w", err)
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
		if err != nil {
			return fmt.Errorf("failed to read block tx count: %w", err)
		}
		blockTxCounts[i] = blockTxCount
		totalBlockTxCount += blockTxCount
	}
	txDataHeaders := make([]uint64, totalBlockTxCount)
	for i := 0; i < int(totalBlockTxCount); i++ {
		txDataHeader, err := binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("failed to read tx data header: %w", err)
		}
		txDataHeaders[i] = txDataHeader
	}
	var txDatas []hexutil.Bytes
	for _, txDataHeader := range txDataHeaders {
		txData := make([]byte, txDataHeader)
		_, err = io.ReadFull(r, txData)
		if err != nil {
			return fmt.Errorf("failed to tx data: %w", err)
		}
		txDatas = append(txDatas, txData)
	}
	var txSigs []BatchV2Signature
	for i := 0; i < int(totalBlockTxCount); i++ {
		var txSig BatchV2Signature
		v, err := binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("failed to read tx sig v: %w", err)
		}
		txSig.V = v
		sigBuffer := make([]byte, 32)
		_, err = io.ReadFull(r, sigBuffer)
		if err != nil {
			return fmt.Errorf("failed to read tx sig r: %w", err)
		}
		txSig.R, _ = uint256.FromBig(new(big.Int).SetBytes(sigBuffer))
		_, err = io.ReadFull(r, sigBuffer)
		if err != nil {
			return fmt.Errorf("failed to read tx sig s: %w", err)
		}
		txSig.S, _ = uint256.FromBig(new(big.Int).SetBytes(sigBuffer))
		txSigs = append(txSigs, txSig)
	}
	b.BlockCount = blockCount
	b.SetOriginBits(originBitBuffer, blockCount)
	b.BlockTxCounts = blockTxCounts
	b.TxDataHeaders = txDataHeaders
	b.TxDatas = txDatas
	b.TxSigs = txSigs
	return nil
}

// DecodeBytes parses data into b from data
func (b *BatchV2) DecodeBytes(data []byte) error {
	if err := b.DecodePrefix(data[:BatchV2PrefixLen]); err != nil {
		return err
	}
	if err := b.DecodePayload(data[BatchV2PrefixLen:]); err != nil {
		return err
	}
	return nil
}

func (b *BatchV2) EncodePrefix(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, b.Timestamp); err != nil {
		return fmt.Errorf("cannot write timestamp: %w", err)
	}
	if _, err := w.Write(b.ParentCheck); err != nil {
		return fmt.Errorf("cannot write parent check: %w", err)
	}
	if _, err := w.Write(b.L1OriginCheck); err != nil {
		return fmt.Errorf("cannot write l1 origin check: %w", err)
	}
	return nil
}

func (b *BatchV2) EncodePayload(w io.Writer) error {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], b.BlockCount)
	if _, err := w.Write(buf[:n]); err != nil {
		return fmt.Errorf("cannot write block count: %w", err)
	}
	if _, err := w.Write(b.OriginBits.Bytes()); err != nil {
		return fmt.Errorf("cannot write origin bits: %w", err)
	}
	for _, blockTxCount := range b.BlockTxCounts {
		n = binary.PutUvarint(buf[:], blockTxCount)
		if _, err := w.Write(buf[:n]); err != nil {
			return fmt.Errorf("cannot write block tx count: %w", err)
		}
	}
	for _, txDataHeader := range b.TxDataHeaders {
		n = binary.PutUvarint(buf[:], txDataHeader)
		if _, err := w.Write(buf[:n]); err != nil {
			return fmt.Errorf("cannot write block tx data header: %w", err)
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
		if _, err := w.Write(txSig.R.Bytes()); err != nil {
			return fmt.Errorf("cannot write tx sig r: %w", err)
		}
		if _, err := w.Write(txSig.S.Bytes()); err != nil {
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
		return rlp.DecodeBytes(data[1:], &b.BatchV1)
	case BatchV2Type:
		return b.BatchV2.DecodeBytes(data[1:])
	default:
		return fmt.Errorf("unrecognized batch type: %d", data[0])
	}
}

package derive

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
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
	SpanBatchV2Type
)

type Batch interface {
	GetBatchType() int
	GetTimestamp() uint64
	GetEpochNum() rollup.Epoch
	GetLogContext(log.Logger) log.Logger
	CheckOriginHash(common.Hash) bool
	CheckParentHash(common.Hash) bool
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
	case SpanBatchType, SpanBatchV2Type:
		buf.WriteByte(byte(b.BatchType))
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
	case SpanBatchType, SpanBatchV2Type:
		b.BatchType = int(data[0])
		b.SpanBatch.batchType = int(data[0])
		return b.SpanBatch.DecodeBytes(data[1:])
	default:
		return fmt.Errorf("unrecognized batch type: %d", data[0])
	}
}

func (b *BatchData) GetBatch() (Batch, error) {
	switch b.BatchType {
	case SingularBatchType:
		return &b.SingularBatch, nil
	case SpanBatchType, SpanBatchV2Type:
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

func NewSpanBatchData(spanBatch SpanBatch, batchType int) *BatchData {
	return &BatchData{
		BatchType: batchType,
		SpanBatch: spanBatch,
	}
}

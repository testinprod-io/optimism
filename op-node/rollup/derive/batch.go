package derive

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

// Batch format
// first byte is type followed by bytestring.
//
// An empty input is not a valid batch.
//
// Note: the type system is based on L1 typed transactions.
//
// encodeBufferPool holds temporary encoder buffers for batch encoding
var encodeBufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

const (
	// SingularBatchType is the first version of Batch format, representing a single L2 block.
	SingularBatchType = 0
	// SpanBatchType is the Batch version used after SpanBatch hard fork, representing a span of L2 blocks.
	SpanBatchType = 1
)

// Batch contains information to build one or multiple L2 blocks.
// Batcher converts L2 blocks into Batch and writes encoded bytes to Channel.
// Derivation pipeline decodes Batch from Channel, and converts to one or multiple payload attributes.
type Batch interface {
	GetBatchType() int
	GetTimestamp() uint64
	LogContext(log.Logger) log.Logger
}

// BatchData is used to represent the typed encoding & decoding.
// and wraps arount a single interface InnerBatchData.
type BatchData struct {
	inner InnerBatchData
}

// InnerBatchData is the underlying data of a BatchData.
// This is implemented by SingularBatch and RawSpanBatch.
type InnerBatchData interface {
	GetBatchType() int
	encode(w io.Writer) error
	decode(r *bytes.Reader) error
	encodeBytes() ([]byte, error)
	decodeBytes(data []byte) error
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

func (bd *BatchData) GetBatchType() uint8 {
	return uint8(bd.inner.GetBatchType())
}

// Encode writes the byte encoding of BatchData to Writer stream
func (bd *BatchData) Encode(w io.Writer) error {
	return bd.inner.encode(w)
}

// Decode reads the byte encoding of BatchData from Reader stream
func (bd *BatchData) Decode(r *bytes.Reader) error {
	return bd.inner.decode(r)
}

// EncodeBytes returns the byte encoding of BatchData
func (bd *BatchData) EncodeBytes() ([]byte, error) {
	return bd.inner.encodeBytes()
}

// EecodeBytes parses data into bd from data
func (bd *BatchData) DecodeBytes(data []byte) error {
	return bd.inner.decodeBytes(data)
}

// MarshalBinary returns the canonical encoding of the batch.
func (b *BatchData) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	err := b.encodeTyped(&buf)
	return buf.Bytes(), err
}

// encodeTyped encodes batch type and payload for each batch type.
func (b *BatchData) encodeTyped(buf *bytes.Buffer) error {
	if err := buf.WriteByte(b.GetBatchType()); err != nil {
		return err
	}
	return b.Encode(buf)
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

// decodeTyped decodes a typed batchData
func (b *BatchData) decodeTyped(data []byte) error {
	if len(data) == 0 {
		return errors.New("batch too short")
	}
	switch data[0] {
	case SingularBatchType:
		var inner SingularBatch
		b.inner = &inner
		if err := b.DecodeBytes(data[1:]); err != nil {
			return err
		}
		return nil
	case SpanBatchType:
		var inner RawSpanBatch
		b.inner = &inner
		if err := b.DecodeBytes(data[1:]); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unrecognized batch type: %d", data[0])
	}
}

// NewBatchData creates a new BatchData
func NewBatchData(inner InnerBatchData) *BatchData {
	return &BatchData{inner: inner}
}

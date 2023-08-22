package derive

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

type SpanBatchTxData interface {
	txType() byte // returns the type ID
}

type SpanBatchTx struct {
	inner SpanBatchTxData
}

type SpanBatchLegacyTxData struct {
	Nonce    uint64          // nonce of sender account
	GasPrice *big.Int        // wei per gas
	Gas      uint64          // gas limit
	To       *common.Address `rlp:"nil"` // nil means contract creation
	Value    *big.Int        // wei amount
	Data     []byte          // contract invocation input data
}

func (txData SpanBatchLegacyTxData) txType() byte { return types.LegacyTxType }

type SpanBatchAccessListTxData struct {
	ChainID    *big.Int         // destination chain ID
	Nonce      uint64           // nonce of sender account
	GasPrice   *big.Int         // wei per gas
	Gas        uint64           // gas limit
	To         *common.Address  `rlp:"nil"` // nil means contract creation
	Value      *big.Int         // wei amount
	Data       []byte           // contract invocation input data
	AccessList types.AccessList // EIP-2930 access list
}

func (txData SpanBatchAccessListTxData) txType() byte { return types.AccessListTxType }

type SpanBatchDynamicFeeTxData struct {
	ChainID    *big.Int
	Nonce      uint64
	GasTipCap  *big.Int // a.k.a. maxPriorityFeePerGas
	GasFeeCap  *big.Int // a.k.a. maxFeePerGas
	Gas        uint64
	To         *common.Address `rlp:"nil"` // nil means contract creation
	Value      *big.Int
	Data       []byte
	AccessList types.AccessList
}

func (txData SpanBatchDynamicFeeTxData) txType() byte { return types.DynamicFeeTxType }

// Type returns the transaction type.
func (tx *SpanBatchTx) Type() uint8 {
	return tx.inner.txType()
}

// encodeTyped writes the canonical encoding of a typed transaction to w.
func (tx *SpanBatchTx) encodeTyped(w *bytes.Buffer) error {
	w.WriteByte(tx.Type())
	return rlp.Encode(w, tx.inner)
}

// MarshalBinary returns the canonical encoding of the transaction.
// For legacy transactions, it returns the RLP encoding. For EIP-2718 typed
// transactions, it returns the type and payload.
func (tx *SpanBatchTx) MarshalBinary() ([]byte, error) {
	if tx.Type() == types.LegacyTxType {
		return rlp.EncodeToBytes(tx.inner)
	}
	var buf bytes.Buffer
	err := tx.encodeTyped(&buf)
	return buf.Bytes(), err
}

// EncodeRLP implements rlp.Encoder
func (tx *SpanBatchTx) EncodeRLP(w io.Writer) error {
	if tx.Type() == types.LegacyTxType {
		return rlp.Encode(w, tx.inner)
	}
	// It's an EIP-2718 typed TX envelope.
	buf := encodeBufferPool.Get().(*bytes.Buffer)
	defer encodeBufferPool.Put(buf)
	buf.Reset()
	if err := tx.encodeTyped(buf); err != nil {
		return err
	}
	return rlp.Encode(w, buf.Bytes())
}

// setDecoded sets the inner transaction after decoding.
func (tx *SpanBatchTx) setDecoded(inner SpanBatchTxData, size uint64) {
	tx.inner = inner
}

// decodeTyped decodes a typed transaction from the canonical format.
func (tx *SpanBatchTx) decodeTyped(b []byte) (SpanBatchTxData, error) {
	if len(b) <= 1 {
		return nil, errors.New("typed transaction too short")
	}
	switch b[0] {
	case types.AccessListTxType:
		var inner SpanBatchAccessListTxData
		err := rlp.DecodeBytes(b[1:], &inner)
		return &inner, err
	case types.DynamicFeeTxType:
		var inner SpanBatchDynamicFeeTxData
		err := rlp.DecodeBytes(b[1:], &inner)
		return &inner, err
	default:
		return nil, types.ErrTxTypeNotSupported
	}
}

// DecodeRLP implements rlp.Decoder
func (tx *SpanBatchTx) DecodeRLP(s *rlp.Stream) error {
	kind, size, err := s.Kind()
	switch {
	case err != nil:
		return err
	case kind == rlp.List:
		// It's a legacy transaction.
		var inner SpanBatchLegacyTxData
		err := s.Decode(&inner)
		if err == nil {
			tx.setDecoded(&inner, rlp.ListSize(size))
		}
		return err
	default:
		// It's an EIP-2718 typed TX envelope.
		var b []byte
		if b, err = s.Bytes(); err != nil {
			return err
		}
		inner, err := tx.decodeTyped(b)
		if err == nil {
			tx.setDecoded(inner, uint64(len(b)))
		}
		return err
	}
}

// UnmarshalBinary decodes the canonical encoding of transactions.
// It supports legacy RLP transactions and EIP2718 typed transactions.
func (tx *SpanBatchTx) UnmarshalBinary(b []byte) error {
	if len(b) > 0 && b[0] > 0x7f {
		// It's a legacy transaction.
		var data SpanBatchLegacyTxData
		err := rlp.DecodeBytes(b, &data)
		if err != nil {
			return err
		}
		tx.setDecoded(&data, uint64(len(b)))
		return nil
	}
	// It's an EIP2718 typed transaction envelope.
	inner, err := tx.decodeTyped(b)
	if err != nil {
		return err
	}
	tx.setDecoded(inner, uint64(len(b)))
	return nil
}

// ConvertToFullTx takes signature value and convert SpanBatchTx to types.Transaction
func (tx SpanBatchTx) ConvertToFullTx(V, R, S *big.Int) (*types.Transaction, error) {
	var inner types.TxData
	switch tx.Type() {
	case types.LegacyTxType:
		batchTxInner := tx.inner.(*SpanBatchLegacyTxData)
		inner = &types.LegacyTx{
			Nonce:    batchTxInner.Nonce,
			GasPrice: batchTxInner.GasPrice,
			Gas:      batchTxInner.Gas,
			To:       batchTxInner.To,
			Value:    batchTxInner.Value,
			Data:     batchTxInner.Data,
			V:        V,
			R:        R,
			S:        S,
		}
	case types.AccessListTxType:
		batchTxInner := tx.inner.(*SpanBatchAccessListTxData)
		inner = &types.AccessListTx{
			ChainID:    batchTxInner.ChainID,
			Nonce:      batchTxInner.Nonce,
			GasPrice:   batchTxInner.GasPrice,
			Gas:        batchTxInner.Gas,
			To:         batchTxInner.To,
			Value:      batchTxInner.Value,
			Data:       batchTxInner.Data,
			AccessList: batchTxInner.AccessList,
			V:          V,
			R:          R,
			S:          S,
		}
	case types.DynamicFeeTxType:
		batchTxInner := tx.inner.(*SpanBatchDynamicFeeTxData)
		inner = &types.DynamicFeeTx{
			ChainID:    batchTxInner.ChainID,
			Nonce:      batchTxInner.Nonce,
			GasTipCap:  batchTxInner.GasTipCap,
			GasFeeCap:  batchTxInner.GasFeeCap,
			Gas:        batchTxInner.Gas,
			To:         batchTxInner.To,
			Value:      batchTxInner.Value,
			Data:       batchTxInner.Data,
			AccessList: batchTxInner.AccessList,
			V:          V,
			R:          R,
			S:          S,
		}
	default:
		return nil, fmt.Errorf("invalid tx type: %d", tx.Type())
	}
	return types.NewTx(inner), nil
}

// NewSpanBatchTx creates a new spanBatch transaction.
func NewSpanBatchTx(tx types.Transaction) (*SpanBatchTx, error) {
	var inner SpanBatchTxData
	switch tx.Type() {
	case types.LegacyTxType:
		inner = SpanBatchLegacyTxData{
			Nonce:    tx.Nonce(),
			GasPrice: tx.GasPrice(),
			Gas:      tx.Gas(),
			To:       tx.To(),
			Value:    tx.Value(),
			Data:     tx.Data(),
		}
	case types.AccessListTxType:
		inner = SpanBatchAccessListTxData{
			ChainID:    tx.ChainId(),
			Nonce:      tx.Nonce(),
			GasPrice:   tx.GasPrice(),
			Gas:        tx.Gas(),
			To:         tx.To(),
			Value:      tx.Value(),
			Data:       tx.Data(),
			AccessList: tx.AccessList(),
		}
	case types.DynamicFeeTxType:
		inner = SpanBatchDynamicFeeTxData{
			ChainID:    tx.ChainId(),
			Nonce:      tx.Nonce(),
			GasTipCap:  tx.GasTipCap(),
			GasFeeCap:  tx.GasFeeCap(),
			Gas:        tx.Gas(),
			To:         tx.To(),
			Value:      tx.Value(),
			Data:       tx.Data(),
			AccessList: tx.AccessList(),
		}
	default:
		return nil, fmt.Errorf("invalid tx type: %d", tx.Type())
	}
	return &SpanBatchTx{inner: inner}, nil
}

package derive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
)

type BatchV2Signature struct {
	V uint64
	R *uint256.Int
	S *uint256.Int
}

type BatchV2TxsV1 struct {
	// below single field must be manually set
	TotalBlockTxCount uint64

	TxDatas []hexutil.Bytes
	TxSigs  []BatchV2Signature
}

func (btx *BatchV2TxsV1) Encode(w io.Writer) error {
	var buf [binary.MaxVarintLen64]byte
	for _, txData := range btx.TxDatas {
		if _, err := w.Write(txData); err != nil {
			return fmt.Errorf("cannot write block tx data: %w", err)
		}
	}
	for _, txSig := range btx.TxSigs {
		n := binary.PutUvarint(buf[:], txSig.V)
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

func (btx *BatchV2TxsV1) Decode(r *bytes.Reader) error {
	// Do not need txDataHeader because RLP byte stream already includes length info
	txDatas := make([]hexutil.Bytes, btx.TotalBlockTxCount)
	for i := 0; i < int(btx.TotalBlockTxCount); i++ {
		txData, _, err := ReadTxData(r)
		if err != nil {
			return err
		}
		txDatas[i] = txData
	}

	txSigs := make([]BatchV2Signature, btx.TotalBlockTxCount)
	var sigBuffer [32]byte
	for i := 0; i < int(btx.TotalBlockTxCount); i++ {
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
	btx.TxDatas = txDatas
	btx.TxSigs = txSigs
	return nil
}

func (btx *BatchV2TxsV1) FullTxs() ([][]byte, error) {
	var txs [][]byte
	for idx := 0; idx < int(btx.TotalBlockTxCount); idx++ {
		var batchV2Tx BatchV2Tx
		if err := batchV2Tx.UnmarshalBinary(btx.TxDatas[idx]); err != nil {
			return nil, err
		}
		v := new(big.Int).SetUint64(btx.TxSigs[idx].V)
		r := btx.TxSigs[idx].R.ToBig()
		s := btx.TxSigs[idx].S.ToBig()
		tx, err := batchV2Tx.ConvertToFullTx(v, r, s)
		if err != nil {
			return nil, err
		}
		encodedTx, err := tx.MarshalBinary()
		if err != nil {
			return nil, err
		}
		txs = append(txs, encodedTx)
	}
	return txs, nil
}

func NewBatchV2TxsV1(txs [][]byte) (*BatchV2TxsV1, error) {
	totalBlockTxCount := uint64(len(txs))
	var txDatas []hexutil.Bytes
	var txSigs []BatchV2Signature
	for idx := 0; idx < int(totalBlockTxCount); idx++ {
		var tx types.Transaction
		if err := tx.UnmarshalBinary(txs[idx]); err != nil {
			return nil, errors.New("failed to decode tx")
		}
		var txSig BatchV2Signature
		v, r, s := tx.RawSignatureValues()
		R, _ := uint256.FromBig(r)
		S, _ := uint256.FromBig(s)
		txSig.V = v.Uint64()
		txSig.R = R
		txSig.S = S
		txSigs = append(txSigs, txSig)
		batchV2TxV1, err := NewBatchV2TxV1(tx)
		if err != nil {
			return nil, err
		}
		txData, err := batchV2TxV1.MarshalBinary()
		if err != nil {
			return nil, err
		}
		txDatas = append(txDatas, txData)
	}
	return &BatchV2TxsV1{
		TotalBlockTxCount: totalBlockTxCount,
		TxDatas:           txDatas,
		TxSigs:            txSigs,
	}, nil
}

type BatchV2TxData interface {
	txType() byte // returns the type ID
}

type BatchV2Tx struct {
	inner BatchV2TxData
}

type BatchV2LegacyTxData struct {
	Nonce    uint64          // nonce of sender account
	GasPrice *big.Int        // wei per gas
	Gas      uint64          // gas limit
	To       *common.Address `rlp:"nil"` // nil means contract creation
	Value    *big.Int        // wei amount
	Data     []byte          // contract invocation input data
}

func (txData BatchV2LegacyTxData) txType() byte { return types.LegacyTxType }

type BatchV2AccessListTxData struct {
	ChainID    *big.Int         // destination chain ID
	Nonce      uint64           // nonce of sender account
	GasPrice   *big.Int         // wei per gas
	Gas        uint64           // gas limit
	To         *common.Address  `rlp:"nil"` // nil means contract creation
	Value      *big.Int         // wei amount
	Data       []byte           // contract invocation input data
	AccessList types.AccessList // EIP-2930 access list
}

func (txData BatchV2AccessListTxData) txType() byte { return types.AccessListTxType }

type BatchV2DynamicFeeTxData struct {
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

func (txData BatchV2DynamicFeeTxData) txType() byte { return types.DynamicFeeTxType }

// Type returns the transaction type.
func (tx *BatchV2Tx) Type() uint8 {
	return tx.inner.txType()
}

// encodeTyped writes the canonical encoding of a typed transaction to w.
func (tx *BatchV2Tx) encodeTyped(w *bytes.Buffer) error {
	w.WriteByte(tx.Type())
	return rlp.Encode(w, tx.inner)
}

// MarshalBinary returns the canonical encoding of the transaction.
// For legacy transactions, it returns the RLP encoding. For EIP-2718 typed
// transactions, it returns the type and payload.
func (tx *BatchV2Tx) MarshalBinary() ([]byte, error) {
	if tx.Type() == types.LegacyTxType {
		return rlp.EncodeToBytes(tx.inner)
	}
	var buf bytes.Buffer
	err := tx.encodeTyped(&buf)
	return buf.Bytes(), err
}

// EncodeRLP implements rlp.Encoder
func (tx *BatchV2Tx) EncodeRLP(w io.Writer) error {
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
func (tx *BatchV2Tx) setDecoded(inner BatchV2TxData, size uint64) {
	tx.inner = inner
}

// decodeTyped decodes a typed transaction from the canonical format.
func (tx *BatchV2Tx) decodeTyped(b []byte) (BatchV2TxData, error) {
	if len(b) <= 1 {
		return nil, errors.New("typed transaction too short")
	}
	switch b[0] {
	case types.AccessListTxType:
		var inner BatchV2AccessListTxData
		err := rlp.DecodeBytes(b[1:], &inner)
		return &inner, err
	case types.DynamicFeeTxType:
		var inner BatchV2DynamicFeeTxData
		err := rlp.DecodeBytes(b[1:], &inner)
		return &inner, err
	default:
		return nil, types.ErrTxTypeNotSupported
	}
}

// DecodeRLP implements rlp.Decoder
func (tx *BatchV2Tx) DecodeRLP(s *rlp.Stream) error {
	kind, size, err := s.Kind()
	switch {
	case err != nil:
		return err
	case kind == rlp.List:
		// It's a legacy transaction.
		var inner BatchV2LegacyTxData
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
func (tx *BatchV2Tx) UnmarshalBinary(b []byte) error {
	if len(b) > 0 && b[0] > 0x7f {
		// It's a legacy transaction.
		var data BatchV2LegacyTxData
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

// ConvertToFullTx takes signature value and convert BatchV2Tx to types.Transaction
func (tx BatchV2Tx) ConvertToFullTx(V, R, S *big.Int) (*types.Transaction, error) {
	var inner types.TxData
	switch tx.Type() {
	case types.LegacyTxType:
		batchTxInner := tx.inner.(*BatchV2LegacyTxData)
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
		batchTxInner := tx.inner.(*BatchV2AccessListTxData)
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
		batchTxInner := tx.inner.(*BatchV2DynamicFeeTxData)
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

// NewBatchV2TxV1 creates a new batchV2 transaction with V1 tx encoding.
func NewBatchV2TxV1(tx types.Transaction) (*BatchV2Tx, error) {
	var inner BatchV2TxData
	switch tx.Type() {
	case types.LegacyTxType:
		inner = BatchV2LegacyTxData{
			Nonce:    tx.Nonce(),
			GasPrice: tx.GasPrice(),
			Gas:      tx.Gas(),
			To:       tx.To(),
			Value:    tx.Value(),
			Data:     tx.Data(),
		}
	case types.AccessListTxType:
		inner = BatchV2AccessListTxData{
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
		inner = BatchV2DynamicFeeTxData{
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
	return &BatchV2Tx{inner: inner}, nil
}

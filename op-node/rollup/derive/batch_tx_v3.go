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

type BatchV2TxsV3 struct {
	// these two fields must be manually set
	TotalBlockTxCount uint64
	ChainID           *big.Int

	// 7 fields
	ContractCreationBits *big.Int
	YParityBits          *big.Int
	TxSigs               []BatchV2Signature
	TxNonces             []uint64
	TxGases              []uint64
	TxTos                []common.Address
	TxDatas              []hexutil.Bytes
}

func (btx *BatchV2TxsV3) EncodeContractCreationBits(w io.Writer) error {
	contractCreationBitBufferLen := btx.TotalBlockTxCount / 8
	if btx.TotalBlockTxCount%8 != 0 {
		contractCreationBitBufferLen++
	}
	contractCreationBitBuffer := make([]byte, contractCreationBitBufferLen)
	for i := 0; i < int(btx.TotalBlockTxCount); i += 8 {
		end := i + 8
		if end < int(btx.TotalBlockTxCount) {
			end = int(btx.TotalBlockTxCount)
		}
		var bits uint = 0
		for j := i; j < end; j++ {
			bits |= btx.ContractCreationBits.Bit(j) << (j - i)
		}
		contractCreationBitBuffer[i/8] = byte(bits)
	}
	if _, err := w.Write(contractCreationBitBuffer); err != nil {
		return fmt.Errorf("cannot write contract creation bits: %w", err)
	}
	return nil
}

func (btx *BatchV2TxsV3) DecodeContractCreationBits(r *bytes.Reader) error {
	contractCreationBitBufferLen := btx.TotalBlockTxCount / 8
	if btx.TotalBlockTxCount%8 != 0 {
		contractCreationBitBufferLen++
	}
	contractCreationBitBuffer := make([]byte, contractCreationBitBufferLen)
	_, err := io.ReadFull(r, contractCreationBitBuffer)
	if err != nil {
		return fmt.Errorf("failed to read contract creation bits: %w", err)
	}
	contractCreationBits := new(big.Int)
	for i := 0; i < int(btx.TotalBlockTxCount); i += 8 {
		end := i + 8
		if end < int(btx.TotalBlockTxCount) {
			end = int(btx.TotalBlockTxCount)
		}
		bits := contractCreationBitBuffer[i/8]
		for j := i; j < end; j++ {
			bit := uint((bits >> (j - i)) & 1)
			contractCreationBits.SetBit(contractCreationBits, j, bit)
		}
	}
	btx.ContractCreationBits = contractCreationBits
	return nil
}

func (btx *BatchV2TxsV3) ContractCreationCount() uint64 {
	if btx.ContractCreationBits == nil {
		panic("contract creation bits not set")
	}
	var result uint64 = 0
	for i := 0; i < int(btx.TotalBlockTxCount); i++ {
		bit := btx.ContractCreationBits.Bit(i)
		if bit == 1 {
			result++
		}
	}
	return result
}

func (btx *BatchV2TxsV3) EncodeYParityBits(w io.Writer) error {
	yParityBitBufferLen := btx.TotalBlockTxCount / 8
	if btx.TotalBlockTxCount%8 != 0 {
		yParityBitBufferLen++
	}
	yParityBitBuffer := make([]byte, yParityBitBufferLen)
	for i := 0; i < int(btx.TotalBlockTxCount); i += 8 {
		end := i + 8
		if end < int(btx.TotalBlockTxCount) {
			end = int(btx.TotalBlockTxCount)
		}
		var bits uint = 0
		for j := i; j < end; j++ {
			bits |= btx.YParityBits.Bit(j) << (j - i)
		}
		yParityBitBuffer[i/8] = byte(bits)
	}
	if _, err := w.Write(yParityBitBuffer); err != nil {
		return fmt.Errorf("cannot write y parity bits: %w", err)
	}
	return nil
}

func (btx *BatchV2TxsV3) EncodeTxSigs(w io.Writer) error {
	for _, txSig := range btx.TxSigs {
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

func (btx *BatchV2TxsV3) EncodeTxNonces(w io.Writer) error {
	var buf [binary.MaxVarintLen64]byte
	for _, txNonce := range btx.TxNonces {
		n := binary.PutUvarint(buf[:], txNonce)
		if _, err := w.Write(buf[:n]); err != nil {
			return fmt.Errorf("cannot write tx nonce: %w", err)
		}
	}
	return nil
}

func (btx *BatchV2TxsV3) EncodeTxGases(w io.Writer) error {
	var buf [binary.MaxVarintLen64]byte
	for _, txGas := range btx.TxGases {
		n := binary.PutUvarint(buf[:], txGas)
		if _, err := w.Write(buf[:n]); err != nil {
			return fmt.Errorf("cannot write tx gas: %w", err)
		}
	}
	return nil
}

func (btx *BatchV2TxsV3) EncodeTxTos(w io.Writer) error {
	for _, txTo := range btx.TxTos {
		if _, err := w.Write(txTo.Bytes()); err != nil {
			return fmt.Errorf("cannot write tx to address: %w", err)
		}
	}
	return nil
}

func (btx *BatchV2TxsV3) EncodeTxDatas(w io.Writer) error {
	for _, txData := range btx.TxDatas {
		if _, err := w.Write(txData); err != nil {
			return fmt.Errorf("cannot write tx data: %w", err)
		}
	}
	return nil
}

func (btx *BatchV2TxsV3) DecodeYParityBits(r *bytes.Reader) error {
	yParityBitBufferLen := btx.TotalBlockTxCount / 8
	if btx.TotalBlockTxCount%8 != 0 {
		yParityBitBufferLen++
	}
	yParityBitBuffer := make([]byte, yParityBitBufferLen)
	_, err := io.ReadFull(r, yParityBitBuffer)
	if err != nil {
		return fmt.Errorf("failed to read y parity bits: %w", err)
	}
	yParityBits := new(big.Int)
	for i := 0; i < int(btx.TotalBlockTxCount); i += 8 {
		end := i + 8
		if end < int(btx.TotalBlockTxCount) {
			end = int(btx.TotalBlockTxCount)
		}
		bits := yParityBitBuffer[i/8]
		for j := i; j < end; j++ {
			bit := uint((bits >> (j - i)) & 1)
			yParityBits.SetBit(yParityBits, j, bit)
		}
	}
	btx.YParityBits = yParityBits
	return nil
}

func (btx *BatchV2TxsV3) DecodeTxSigs(r *bytes.Reader) error {
	txSigs := make([]BatchV2Signature, btx.TotalBlockTxCount)
	var sigBuffer [32]byte
	for i := 0; i < int(btx.TotalBlockTxCount); i++ {
		var txSig BatchV2Signature
		_, err := io.ReadFull(r, sigBuffer[:])
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
	btx.TxSigs = txSigs
	return nil
}

func (btx *BatchV2TxsV3) DecodeTxNonces(r *bytes.Reader) error {
	txNonces := make([]uint64, btx.TotalBlockTxCount)
	for i := 0; i < int(btx.TotalBlockTxCount); i++ {
		txNonce, err := binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("failed to read tx nonce: %w", err)
		}
		txNonces[i] = txNonce
	}
	btx.TxNonces = txNonces
	return nil
}

func (btx *BatchV2TxsV3) DecodeTxGases(r *bytes.Reader) error {
	txGases := make([]uint64, btx.TotalBlockTxCount)
	for i := 0; i < int(btx.TotalBlockTxCount); i++ {
		txGas, err := binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("failed to read tx gas: %w", err)
		}
		txGases[i] = txGas
	}
	btx.TxGases = txGases
	return nil
}

func (btx *BatchV2TxsV3) DecodeTxTos(r *bytes.Reader) error {
	var txTos []common.Address
	txToBuffer := make([]byte, common.AddressLength)
	contractCreationCount := btx.ContractCreationCount()
	for i := 0; i < int(btx.TotalBlockTxCount-contractCreationCount); i++ {
		_, err := io.ReadFull(r, txToBuffer)
		if err != nil {
			return fmt.Errorf("failed to read tx to address: %w", err)
		}
		txTos = append(txTos, common.BytesToAddress(txToBuffer))
	}
	btx.TxTos = txTos
	return nil
}

func (btx *BatchV2TxsV3) DecodeTxDatas(r *bytes.Reader) ([]int, error) {
	txDatas := make([]hexutil.Bytes, btx.TotalBlockTxCount)
	txTypes := make([]int, btx.TotalBlockTxCount)
	// Do not need txDataHeader because RLP byte stream already includes length info
	for i := 0; i < int(btx.TotalBlockTxCount); i++ {
		txData, txType, err := ReadTxData(r)
		if err != nil {
			return nil, err
		}
		txDatas[i] = txData
		txTypes[i] = txType
	}
	btx.TxDatas = txDatas
	return txTypes, nil
}

func (btx *BatchV2TxsV3) RecoverV(txTypes []int) {
	if len(txTypes) != len(btx.TxSigs) {
		panic("tx type length and tx sigs length mismatch")
	}
	for idx, txType := range txTypes {
		bit := uint64(btx.YParityBits.Bit(idx))
		var v uint64
		switch txType {
		case types.LegacyTxType:
			// EIP155
			v = btx.ChainID.Uint64()*2 + 35 + bit
		case types.AccessListTxType:
			v = bit
		case types.DynamicFeeTxType:
			v = bit
		default:
			panic(fmt.Sprintf("invalid tx type: %d", txType))
		}
		btx.TxSigs[idx].V = v
	}
}

func (btx *BatchV2TxsV3) Encode(w io.Writer) error {
	if err := btx.EncodeContractCreationBits(w); err != nil {
		return err
	}
	if err := btx.EncodeYParityBits(w); err != nil {
		return err
	}
	if err := btx.EncodeTxSigs(w); err != nil {
		return err
	}
	if err := btx.EncodeTxNonces(w); err != nil {
		return err
	}
	if err := btx.EncodeTxGases(w); err != nil {
		return err
	}
	// Must be called after EncodeContractCreationBits
	if err := btx.EncodeTxTos(w); err != nil {
		return err
	}
	if err := btx.EncodeTxDatas(w); err != nil {
		return err
	}
	return nil
}

func (btx *BatchV2TxsV3) Decode(r *bytes.Reader) error {
	if err := btx.DecodeContractCreationBits(r); err != nil {
		return err
	}
	if err := btx.DecodeYParityBits(r); err != nil {
		return err
	}
	if err := btx.DecodeTxSigs(r); err != nil {
		return err
	}
	if err := btx.DecodeTxNonces(r); err != nil {
		return err
	}
	if err := btx.DecodeTxGases(r); err != nil {
		return err
	}
	// Must be called after DecodeContractCreationBits
	if err := btx.DecodeTxTos(r); err != nil {
		return err
	}
	txTypes, err := btx.DecodeTxDatas(r)
	if err != nil {
		return err
	}
	// always do last
	btx.RecoverV(txTypes)
	return nil
}

func (btx BatchV2TxsV3) FullTxs() ([][]byte, error) {
	var txs [][]byte
	toIdx := 0
	for idx := 0; idx < int(btx.TotalBlockTxCount); idx++ {
		var batchV2TxV3 BatchV2TxV3
		if err := batchV2TxV3.UnmarshalBinary(btx.TxDatas[idx]); err != nil {
			return nil, err
		}
		nonce := btx.TxNonces[idx]
		gas := btx.TxGases[idx]
		var to *common.Address = nil
		bit := btx.ContractCreationBits.Bit(idx)
		if bit == 0 {
			if len(btx.TxTos) <= toIdx {
				return nil, errors.New("tx to not enough")
			}
			to = &btx.TxTos[toIdx]
			toIdx++
		}
		v := new(big.Int).SetUint64(btx.TxSigs[idx].V)
		r := btx.TxSigs[idx].R.ToBig()
		s := btx.TxSigs[idx].S.ToBig()
		tx, err := batchV2TxV3.ConvertToFullTx(nonce, gas, to, btx.ChainID, v, r, s)
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

func ConvertVToYParity(v uint64, txType int) uint {
	var yParityBit uint
	switch txType {
	case types.LegacyTxType:
		// EIP155: v = 2 * chainID + 35 + yParity
		// v - 35 = yParity (mod 2)
		yParityBit = uint((v - 35) & 1)
	case types.AccessListTxType:
		yParityBit = uint(v)
	case types.DynamicFeeTxType:
		yParityBit = uint(v)
	default:
		panic(fmt.Sprintf("invalid tx type: %d", txType))
	}
	return yParityBit
}

func NewBatchV2TxsV3(txs [][]byte) (*BatchV2TxsV3, error) {
	totalBlockTxCount := uint64(len(txs))
	var txSigs []BatchV2Signature
	var txTos []common.Address
	var txNonces []uint64
	var txGases []uint64
	var txDatas []hexutil.Bytes
	contractCreationBits := new(big.Int)
	yParityBits := new(big.Int)
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
		contractCreationBit := uint(1)
		if tx.To() != nil {
			txTos = append(txTos, *tx.To())
			contractCreationBit = uint(0)
		}
		contractCreationBits.SetBit(contractCreationBits, idx, contractCreationBit)
		yParityBit := ConvertVToYParity(txSig.V, int(tx.Type()))
		yParityBits.SetBit(yParityBits, idx, yParityBit)
		txNonces = append(txNonces, tx.Nonce())
		txGases = append(txGases, tx.Gas())
		batchV2TxV3, err := NewBatchV2TxV3(tx)
		if err != nil {
			return nil, err
		}
		txData, err := batchV2TxV3.MarshalBinary()
		if err != nil {
			return nil, err
		}
		txDatas = append(txDatas, txData)
	}
	return &BatchV2TxsV3{
		TotalBlockTxCount:    totalBlockTxCount,
		ChainID:              ChainID, // TODO: fix hardcoded chainID
		ContractCreationBits: contractCreationBits,
		YParityBits:          yParityBits,
		TxSigs:               txSigs,
		TxNonces:             txNonces,
		TxGases:              txGases,
		TxTos:                txTos,
		TxDatas:              txDatas,
	}, nil
}

type BatchV2TxDataV3 interface {
	txType() byte // returns the type ID
}

type BatchV2TxV3 struct {
	inner BatchV2TxDataV3
}

type BatchV2LegacyTxDataV3 struct {
	Value    *big.Int // wei amount
	GasPrice *big.Int // wei per gas
	Data     []byte
}

func (txData BatchV2LegacyTxDataV3) txType() byte { return types.LegacyTxType }

type BatchV2AccessListTxDataV3 struct {
	Value      *big.Int // wei amount
	GasPrice   *big.Int // wei per gas
	Data       []byte
	AccessList types.AccessList // EIP-2930 access list
}

func (txData BatchV2AccessListTxDataV3) txType() byte { return types.AccessListTxType }

type BatchV2DynamicFeeTxDataV3 struct {
	Value      *big.Int
	GasTipCap  *big.Int // a.k.a. maxPriorityFeePerGas
	GasFeeCap  *big.Int // a.k.a. maxFeePerGas
	Data       []byte
	AccessList types.AccessList
}

func (txData BatchV2DynamicFeeTxDataV3) txType() byte { return types.DynamicFeeTxType }

// Type returns the transaction type.
func (tx *BatchV2TxV3) Type() uint8 {
	return tx.inner.txType()
}

// encodeTyped writes the canonical encoding of a typed transaction to w.
func (tx *BatchV2TxV3) encodeTyped(w *bytes.Buffer) error {
	w.WriteByte(tx.Type())
	return rlp.Encode(w, tx.inner)
}

// MarshalBinary returns the canonical encoding of the transaction.
// For legacy transactions, it returns the RLP encoding. For EIP-2718 typed
// transactions, it returns the type and payload.
func (tx *BatchV2TxV3) MarshalBinary() ([]byte, error) {
	if tx.Type() == types.LegacyTxType {
		return rlp.EncodeToBytes(tx.inner)
	}
	var buf bytes.Buffer
	err := tx.encodeTyped(&buf)
	return buf.Bytes(), err
}

// EncodeRLP implements rlp.Encoder
func (tx *BatchV2TxV3) EncodeRLP(w io.Writer) error {
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
func (tx *BatchV2TxV3) setDecoded(inner BatchV2TxDataV3, size uint64) {
	tx.inner = inner
}

// decodeTyped decodes a typed transaction from the canonical format.
func (tx *BatchV2TxV3) decodeTyped(b []byte) (BatchV2TxDataV3, error) {
	if len(b) <= 1 {
		return nil, errors.New("typed transaction too short")
	}
	switch b[0] {
	case types.AccessListTxType:
		var inner BatchV2AccessListTxDataV3
		err := rlp.DecodeBytes(b[1:], &inner)
		return &inner, err
	case types.DynamicFeeTxType:
		var inner BatchV2DynamicFeeTxDataV3
		err := rlp.DecodeBytes(b[1:], &inner)
		return &inner, err
	default:
		return nil, types.ErrTxTypeNotSupported
	}
}

// DecodeRLP implements rlp.Decoder
func (tx *BatchV2TxV3) DecodeRLP(s *rlp.Stream) error {
	kind, size, err := s.Kind()
	switch {
	case err != nil:
		return err
	case kind == rlp.List:
		// It's a legacy transaction.
		var inner BatchV2LegacyTxDataV3
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
func (tx *BatchV2TxV3) UnmarshalBinary(b []byte) error {
	if len(b) > 0 && b[0] > 0x7f {
		// It's a legacy transaction.
		var data BatchV2LegacyTxDataV3
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

// ConvertToFullTx takes values and convert BatchV2TxV3 to types.Transaction
func (tx BatchV2TxV3) ConvertToFullTx(nonce, gas uint64, to *common.Address, chainID, V, R, S *big.Int) (*types.Transaction, error) {
	var inner types.TxData
	switch tx.Type() {
	case types.LegacyTxType:
		batchTxInner := tx.inner.(*BatchV2LegacyTxDataV3)
		inner = &types.LegacyTx{
			Nonce:    nonce,
			GasPrice: batchTxInner.GasPrice,
			Gas:      gas,
			To:       to,
			Value:    batchTxInner.Value,
			Data:     batchTxInner.Data,
			V:        V,
			R:        R,
			S:        S,
		}
	case types.AccessListTxType:
		batchTxInner := tx.inner.(*BatchV2AccessListTxDataV3)
		inner = &types.AccessListTx{
			ChainID:    chainID,
			Nonce:      nonce,
			GasPrice:   batchTxInner.GasPrice,
			Gas:        gas,
			To:         to,
			Value:      batchTxInner.Value,
			Data:       batchTxInner.Data,
			AccessList: batchTxInner.AccessList,
			V:          V,
			R:          R,
			S:          S,
		}
	case types.DynamicFeeTxType:
		batchTxInner := tx.inner.(*BatchV2DynamicFeeTxDataV3)
		inner = &types.DynamicFeeTx{
			ChainID:    chainID,
			Nonce:      nonce,
			GasTipCap:  batchTxInner.GasTipCap,
			GasFeeCap:  batchTxInner.GasFeeCap,
			Gas:        gas,
			To:         to,
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

func NewBatchV2TxV3(tx types.Transaction) (*BatchV2TxV3, error) {
	var inner BatchV2TxDataV3
	switch tx.Type() {
	case types.LegacyTxType:
		inner = BatchV2LegacyTxDataV3{
			GasPrice: tx.GasPrice(),
			Value:    tx.Value(),
			Data:     tx.Data(),
		}
	case types.AccessListTxType:
		inner = BatchV2AccessListTxDataV3{
			GasPrice:   tx.GasPrice(),
			Value:      tx.Value(),
			Data:       tx.Data(),
			AccessList: tx.AccessList(),
		}
	case types.DynamicFeeTxType:
		inner = BatchV2DynamicFeeTxDataV3{
			GasTipCap:  tx.GasTipCap(),
			GasFeeCap:  tx.GasFeeCap(),
			Value:      tx.Value(),
			Data:       tx.Data(),
			AccessList: tx.AccessList(),
		}
	default:
		return nil, fmt.Errorf("invalid tx type: %d", tx.Type())
	}
	return &BatchV2TxV3{inner: inner}, nil
}

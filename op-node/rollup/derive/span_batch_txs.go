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
	"github.com/holiman/uint256"
)

type SpanBatchTxs struct {
	// these two fields must be manually set
	TotalBlockTxCount uint64
	ChainID           *big.Int

	// 7 fields
	ContractCreationBits *big.Int
	YParityBits          *big.Int
	TxSigs               []SpanBatchSignature
	TxNonces             []uint64
	TxGases              []uint64
	TxTos                []common.Address
	TxDatas              []hexutil.Bytes

	txTypes []int
}

func (btx *SpanBatchTxs) EncodeContractCreationBits(w io.Writer) error {
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

func (btx *SpanBatchTxs) DecodeContractCreationBits(r *bytes.Reader) error {
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

func (btx *SpanBatchTxs) ContractCreationCount() uint64 {
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

func (btx *SpanBatchTxs) EncodeYParityBits(w io.Writer) error {
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

func (btx *SpanBatchTxs) EncodeTxSigs(w io.Writer) error {
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

func (btx *SpanBatchTxs) EncodeTxNonces(w io.Writer) error {
	var buf [binary.MaxVarintLen64]byte
	for _, txNonce := range btx.TxNonces {
		n := binary.PutUvarint(buf[:], txNonce)
		if _, err := w.Write(buf[:n]); err != nil {
			return fmt.Errorf("cannot write tx nonce: %w", err)
		}
	}
	return nil
}

func (btx *SpanBatchTxs) EncodeTxGases(w io.Writer) error {
	var buf [binary.MaxVarintLen64]byte
	for _, txGas := range btx.TxGases {
		n := binary.PutUvarint(buf[:], txGas)
		if _, err := w.Write(buf[:n]); err != nil {
			return fmt.Errorf("cannot write tx gas: %w", err)
		}
	}
	return nil
}

func (btx *SpanBatchTxs) EncodeTxTos(w io.Writer) error {
	for _, txTo := range btx.TxTos {
		if _, err := w.Write(txTo.Bytes()); err != nil {
			return fmt.Errorf("cannot write tx to address: %w", err)
		}
	}
	return nil
}

func (btx *SpanBatchTxs) EncodeTxDatas(w io.Writer) error {
	for _, txData := range btx.TxDatas {
		if _, err := w.Write(txData); err != nil {
			return fmt.Errorf("cannot write tx data: %w", err)
		}
	}
	return nil
}

func (btx *SpanBatchTxs) DecodeYParityBits(r *bytes.Reader) error {
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

func (btx *SpanBatchTxs) DecodeTxSigs(r *bytes.Reader) error {
	txSigs := make([]SpanBatchSignature, btx.TotalBlockTxCount)
	var sigBuffer [32]byte
	for i := 0; i < int(btx.TotalBlockTxCount); i++ {
		var txSig SpanBatchSignature
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

func (btx *SpanBatchTxs) DecodeTxNonces(r *bytes.Reader) error {
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

func (btx *SpanBatchTxs) DecodeTxGases(r *bytes.Reader) error {
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

func (btx *SpanBatchTxs) DecodeTxTos(r *bytes.Reader) error {
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

func (btx *SpanBatchTxs) DecodeTxDatas(r *bytes.Reader) error {
	txDatas := make([]hexutil.Bytes, btx.TotalBlockTxCount)
	txTypes := make([]int, btx.TotalBlockTxCount)
	// Do not need txDataHeader because RLP byte stream already includes length info
	for i := 0; i < int(btx.TotalBlockTxCount); i++ {
		txData, txType, err := ReadTxData(r)
		if err != nil {
			return err
		}
		txDatas[i] = txData
		txTypes[i] = txType
	}
	btx.TxDatas = txDatas
	btx.txTypes = txTypes
	return nil
}

func (btx *SpanBatchTxs) RecoverV() {
	if len(btx.txTypes) != len(btx.TxSigs) {
		panic("tx type length and tx sigs length mismatch")
	}
	for idx, txType := range btx.txTypes {
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

func (btx *SpanBatchTxs) Encode(w io.Writer) error {
	if err := btx.EncodeContractCreationBits(w); err != nil {
		return err
	}
	if err := btx.EncodeYParityBits(w); err != nil {
		return err
	}
	if err := btx.EncodeTxSigs(w); err != nil {
		return err
	}
	if err := btx.EncodeTxTos(w); err != nil {
		return err
	}
	if err := btx.EncodeTxDatas(w); err != nil {
		return err
	}
	if err := btx.EncodeTxNonces(w); err != nil {
		return err
	}
	if err := btx.EncodeTxGases(w); err != nil {
		return err
	}
	return nil
}

func (btx *SpanBatchTxs) Decode(r *bytes.Reader) error {
	if err := btx.DecodeContractCreationBits(r); err != nil {
		return err
	}
	if err := btx.DecodeYParityBits(r); err != nil {
		return err
	}
	if err := btx.DecodeTxSigs(r); err != nil {
		return err
	}
	if err := btx.DecodeTxTos(r); err != nil {
		return err
	}
	err := btx.DecodeTxDatas(r)
	if err != nil {
		return err
	}
	if err := btx.DecodeTxNonces(r); err != nil {
		return err
	}
	if err := btx.DecodeTxGases(r); err != nil {
		return err
	}
	return nil
}

func (btx *SpanBatchTxs) FullTxs() ([][]byte, error) {
	var txs [][]byte
	toIdx := 0
	for idx := 0; idx < int(btx.TotalBlockTxCount); idx++ {
		var spanBatchTx SpanBatchTx
		if err := spanBatchTx.UnmarshalBinary(btx.TxDatas[idx]); err != nil {
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
		tx, err := spanBatchTx.ConvertToFullTx(nonce, gas, to, btx.ChainID, v, r, s)
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

func (btx *SpanBatchTxs) AppendTx(txBytes []byte) error {
	btx.TotalBlockTxCount += 1

	var tx types.Transaction
	if err := tx.UnmarshalBinary(txBytes); err != nil {
		return errors.New("failed to decode tx")
	}
	var txSig SpanBatchSignature
	v, r, s := tx.RawSignatureValues()
	R, _ := uint256.FromBig(r)
	S, _ := uint256.FromBig(s)
	txSig.V = v.Uint64()
	txSig.R = R
	txSig.S = S
	btx.TxSigs = append(btx.TxSigs, txSig)
	contractCreationBit := uint(1)
	if tx.To() != nil {
		btx.TxTos = append(btx.TxTos, *tx.To())
		contractCreationBit = uint(0)
	}
	btx.ContractCreationBits.SetBit(btx.ContractCreationBits, int(btx.TotalBlockTxCount-1), contractCreationBit)
	yParityBit := ConvertVToYParity(txSig.V, int(tx.Type()))
	btx.YParityBits.SetBit(btx.YParityBits, int(btx.TotalBlockTxCount-1), yParityBit)
	btx.TxNonces = append(btx.TxNonces, tx.Nonce())
	btx.TxGases = append(btx.TxGases, tx.Gas())
	spanBatchTx, err := NewSpanBatchTx(tx)
	if err != nil {
		return err
	}
	txData, err := spanBatchTx.MarshalBinary()
	if err != nil {
		return err
	}
	btx.TxDatas = append(btx.TxDatas, txData)
	return nil
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

func NewSpanBatchTxs(txs [][]byte, chainId *big.Int) (*SpanBatchTxs, error) {
	totalBlockTxCount := uint64(len(txs))
	var txSigs []SpanBatchSignature
	var txTos []common.Address
	var txNonces []uint64
	var txGases []uint64
	var txDatas []hexutil.Bytes
	var txTypes []int
	contractCreationBits := new(big.Int)
	yParityBits := new(big.Int)
	for idx := 0; idx < int(totalBlockTxCount); idx++ {
		var tx types.Transaction
		if err := tx.UnmarshalBinary(txs[idx]); err != nil {
			return nil, errors.New("failed to decode tx")
		}
		var txSig SpanBatchSignature
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
		spanBatchTx, err := NewSpanBatchTx(tx)
		if err != nil {
			return nil, err
		}
		txData, err := spanBatchTx.MarshalBinary()
		if err != nil {
			return nil, err
		}
		txDatas = append(txDatas, txData)
		txTypes = append(txTypes, int(tx.Type()))
	}
	return &SpanBatchTxs{
		TotalBlockTxCount:    totalBlockTxCount,
		ChainID:              chainId,
		ContractCreationBits: contractCreationBits,
		YParityBits:          yParityBits,
		TxSigs:               txSigs,
		TxNonces:             txNonces,
		TxGases:              txGases,
		TxTos:                txTos,
		TxDatas:              txDatas,
		txTypes:              txTypes,
	}, nil
}

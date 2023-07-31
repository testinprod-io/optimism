package derive

import (
	"bytes"
	"fmt"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

type LegacyTxNoSig struct {
	Nonce    uint64          // nonce of sender account
	GasPrice *big.Int        // wei per gas
	Gas      uint64          // gas limit
	To       *common.Address `rlp:"nil"` // nil means contract creation
	Value    *big.Int        // wei amount
	Data     []byte          // contract invocation input data
}

type AccessListTxNoSig struct {
	ChainID    *big.Int         // destination chain ID
	Nonce      uint64           // nonce of sender account
	GasPrice   *big.Int         // wei per gas
	Gas        uint64           // gas limit
	To         *common.Address  `rlp:"nil"` // nil means contract creation
	Value      *big.Int         // wei amount
	Data       []byte           // contract invocation input data
	AccessList types.AccessList // EIP-2930 access list
}

type DynamicFeeTxNoSig struct {
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

// TODO: make it zero copy
// TODO: refactor by legacy tx and non legacy tx like geth's transaction.go
// TODO: apply transaction.go like abstraction
func EncodeBatchV2TxData(tx types.Transaction, w io.Writer) error {
	switch tx.Type() {
	case types.LegacyTxType:
		var inner LegacyTxNoSig = LegacyTxNoSig{
			Nonce:    tx.Nonce(),
			GasPrice: tx.GasPrice(),
			Gas:      tx.Gas(),
			To:       tx.To(),
			Value:    tx.Value(),
			Data:     tx.Data(),
		}
		return rlp.Encode(w, inner)
	case types.AccessListTxType:
		var inner AccessListTxNoSig = AccessListTxNoSig{
			ChainID:    tx.ChainId(),
			Nonce:      tx.Nonce(),
			GasPrice:   tx.GasPrice(),
			Gas:        tx.Gas(),
			To:         tx.To(),
			Value:      tx.Value(),
			Data:       tx.Data(),
			AccessList: tx.AccessList(),
		}
		buf := encodeBufferPool.Get().(*bytes.Buffer)
		defer encodeBufferPool.Put(buf)
		buf.Reset()
		buf.WriteByte(tx.Type())
		if err := rlp.Encode(buf, inner); err != nil {
			return err
		}
		return rlp.Encode(w, buf.Bytes())
	case types.DynamicFeeTxType:
		var inner DynamicFeeTxNoSig = DynamicFeeTxNoSig{
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
		buf := encodeBufferPool.Get().(*bytes.Buffer)
		defer encodeBufferPool.Put(buf)
		buf.Reset()
		buf.WriteByte(tx.Type())
		if err := rlp.Encode(buf, inner); err != nil {
			return err
		}
		return rlp.Encode(w, buf.Bytes())
	default:
		// deposit tx is never included in batch
		return fmt.Errorf("invalid tx type: %d", tx.Type())
	}
}

func EncodeBatchV2TxDataBytes(tx types.Transaction) ([]byte, error) {
	buf := encodeBufferPool.Get().(*bytes.Buffer)
	defer encodeBufferPool.Put(buf)
	buf.Reset()
	if err := EncodeBatchV2TxData(tx, buf); err != nil {
		return []byte{}, err
	}
	return buf.Bytes(), nil
}

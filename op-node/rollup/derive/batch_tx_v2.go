package derive

import (
	"bytes"
	"io"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

type BatchV2TxsV2 struct {
	TxDatas [][]byte
}

func (btx *BatchV2TxsV2) Encode(w io.Writer) error {
	return rlp.Encode(w, btx.TxDatas)
}

func (btx *BatchV2TxsV2) Decode(r *bytes.Reader) error {
	return rlp.Decode(r, &btx.TxDatas)
}

// checkTxValid checks rawTx by unmarshaling
func checkTxValid(txs *[][]byte) error {
	for _, rawTx := range *txs {
		var tx types.Transaction
		if err := tx.UnmarshalBinary(rawTx); err != nil {
			return err
		}
	}
	return nil
}

func (btx BatchV2TxsV2) FullTxs() ([][]byte, error) {
	if err := checkTxValid(&btx.TxDatas); err != nil {
		return nil, err
	}
	return btx.TxDatas, nil
}

func NewBatchV2TxsV2(txs [][]byte) (*BatchV2TxsV2, error) {
	if err := checkTxValid(&txs); err != nil {
		return nil, err
	}
	return &BatchV2TxsV2{
		TxDatas: txs,
	}, nil
}

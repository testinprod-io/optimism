package derive

import (
	"bytes"
	"io"
)

type BatchV2TxsV3 struct {
}

func (btx *BatchV2TxsV3) Encode(w io.Writer) error {
	panic("not implemented")
}

func (btx *BatchV2TxsV3) Decode(r *bytes.Reader) error {
	panic("not implemented")
}

func (btx BatchV2TxsV3) FullTxs() ([][]byte, error) {
	panic("not implemented")
}

func NewBatchV2TxsV3(txs [][]byte) (*BatchV2TxsV3, error) {
	panic("not implemented")
}

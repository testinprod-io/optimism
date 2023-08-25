package derive

import (
	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
)

type SingularBatch struct {
	ParentHash common.Hash  // parent L2 block hash
	EpochNum   rollup.Epoch // aka l1 num
	EpochHash  common.Hash  // block hash
	Timestamp  uint64
	// no feeRecipient address input, all fees go to a L2 contract
	Transactions []hexutil.Bytes
}

func (b *SingularBatch) GetBatchType() int {
	return SingularBatchType
}

func (b *SingularBatch) GetTimestamp() uint64 {
	return b.Timestamp
}

func (b *SingularBatch) GetEpochNum() rollup.Epoch {
	return b.EpochNum
}

func (b *SingularBatch) GetLogContext(log log.Logger) log.Logger {
	return log.New(
		"batch_timestamp", b.Timestamp,
		"parent_hash", b.ParentHash,
		"batch_epoch", b.Epoch(),
		"txs", len(b.Transactions),
	)
}

func (b *SingularBatch) CheckOriginHash(hash common.Hash) bool {
	return b.EpochHash == hash
}

func (b *SingularBatch) CheckParentHash(hash common.Hash) bool {
	return b.ParentHash == hash
}

func (b *SingularBatch) Epoch() eth.BlockID {
	return eth.BlockID{Hash: b.EpochHash, Number: uint64(b.EpochNum)}
}

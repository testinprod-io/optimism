package derive

import (
	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
)

// Batch format
//
// SingularBatchType := 0
// singularBatch := SingularBatchType ++ RLP([epoch, timestamp, transaction_list]

// SingularBatch is an implementation of Batch interface, containing the input to build one L2 block.
type SingularBatch struct {
	ParentHash common.Hash  // parent L2 block hash
	EpochNum   rollup.Epoch // aka l1 num
	EpochHash  common.Hash  // block hash
	Timestamp  uint64
	// no feeRecipient address input, all fees go to a L2 contract
	Transactions []hexutil.Bytes
}

// GetBatchType returns its batch type (batch_version)
func (b *SingularBatch) GetBatchType() int {
	return SingularBatchType
}

// GetTimestamp returns its block timestamp
func (b *SingularBatch) GetTimestamp() uint64 {
	return b.Timestamp
}

// GetEpochNum returns its epoch number (L1 origin block number)
func (b *SingularBatch) GetEpochNum() rollup.Epoch {
	return b.EpochNum
}

// LogContext creates a new log context that contains information of the batch
func (b *SingularBatch) LogContext(log log.Logger) log.Logger {
	return log.New(
		"batch_timestamp", b.Timestamp,
		"parent_hash", b.ParentHash,
		"batch_epoch", b.Epoch(),
		"txs", len(b.Transactions),
	)
}

// CheckOriginHash checks if the epoch hash (L1 origin block hash) matches the given hash, probably L1 block hash from the current canonical L1 chain.
func (b *SingularBatch) CheckOriginHash(hash common.Hash) bool {
	return b.EpochHash == hash
}

// CheckParentHash checks if the parent hash matches the given hash, probably the current L2 safe head.
func (b *SingularBatch) CheckParentHash(hash common.Hash) bool {
	return b.ParentHash == hash
}

// Epoch returns a BlockID of its L1 origin.
func (b *SingularBatch) Epoch() eth.BlockID {
	return eth.BlockID{Hash: b.EpochHash, Number: uint64(b.EpochNum)}
}

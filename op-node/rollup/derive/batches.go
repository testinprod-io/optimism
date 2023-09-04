package derive

import (
	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

type BatchWithL1InclusionBlock struct {
	L1InclusionBlock eth.L1BlockRef
	Batch            Batch
}

type BatchValidity uint8

const (
	// BatchDrop indicates that the batch is invalid, and will always be in the future, unless we reorg
	BatchDrop = iota
	// BatchAccept indicates that the batch is valid and should be processed
	BatchAccept
	// BatchUndecided indicates we are lacking L1 information until we can proceed batch filtering
	BatchUndecided
	// BatchFuture indicates that the batch may be valid, but cannot be processed yet and should be checked again later
	BatchFuture
)

func CheckBatch(cfg *rollup.Config, log log.Logger, l1Blocks []eth.L1BlockRef, l2SafeHead eth.L2BlockRef, batch *BatchWithL1InclusionBlock) BatchValidity {
	switch batch.Batch.GetBatchType() {
	case SingularBatchType:
		if cfg.IsSpanBatch(batch.Batch.GetTimestamp()) {
			return BatchDrop
		}
		singularBatch, _ := batch.Batch.(*SingularBatch)
		return CheckSingularBatch(cfg, log, l1Blocks, l2SafeHead, singularBatch, batch.L1InclusionBlock)
	case SpanBatchType, SpanBatchV2Type:
		spanBatch, _ := batch.Batch.(*SpanBatch)
		if !cfg.IsSpanBatch(batch.Batch.GetTimestamp()) {
			return BatchDrop
		}
		return CheckSpanBatch(cfg, log, l1Blocks, l2SafeHead, spanBatch, batch.L1InclusionBlock)
	default:
		log.Warn("unrecognized batch type: %d", batch.Batch.GetBatchType())
		return BatchDrop
	}
}

// CheckBatch checks if the given batch can be applied on top of the given l2SafeHead, given the contextual L1 blocks the batch was included in.
// The first entry of the l1Blocks should match the origin of the l2SafeHead. One or more consecutive l1Blocks should be provided.
// In case of only a single L1 block, the decision whether a batch is valid may have to stay undecided.
func CheckSingularBatch(cfg *rollup.Config, log log.Logger, l1Blocks []eth.L1BlockRef, l2SafeHead eth.L2BlockRef, batch *SingularBatch, l1InclusionBlock eth.L1BlockRef) BatchValidity {
	// add details to the log
	log = batch.GetLogContext(log)

	// sanity check we have consistent inputs
	if len(l1Blocks) == 0 {
		log.Warn("missing L1 block input, cannot proceed with batch checking")
		return BatchUndecided
	}
	epoch := l1Blocks[0]

	nextTimestamp := l2SafeHead.Time + cfg.BlockTime
	if batch.Timestamp > nextTimestamp {
		log.Trace("received out-of-order batch for future processing after next batch", "next_timestamp", nextTimestamp)
		return BatchFuture
	}
	if batch.Timestamp < nextTimestamp {
		log.Warn("dropping batch with old timestamp", "min_timestamp", nextTimestamp)
		return BatchDrop
	}

	// dependent on above timestamp check. If the timestamp is correct, then it must build on top of the safe head.
	if batch.ParentHash != l2SafeHead.Hash {
		log.Warn("ignoring batch with mismatching parent hash", "current_safe_head", l2SafeHead.Hash)
		return BatchDrop
	}

	// Filter out batches that were included too late.
	if uint64(batch.EpochNum)+cfg.SeqWindowSize < l1InclusionBlock.Number {
		log.Warn("batch was included too late, sequence window expired")
		return BatchDrop
	}

	// Check the L1 origin of the batch
	batchOrigin := epoch
	if uint64(batch.EpochNum) < epoch.Number {
		log.Warn("dropped batch, epoch is too old", "minimum", epoch.ID())
		// batch epoch too old
		return BatchDrop
	} else if uint64(batch.EpochNum) == epoch.Number {
		// Batch is sticking to the current epoch, continue.
	} else if uint64(batch.EpochNum) == epoch.Number+1 {
		// With only 1 l1Block we cannot look at the next L1 Origin.
		// Note: This means that we are unable to determine validity of a batch
		// without more information. In this case we should bail out until we have
		// more information otherwise the eager algorithm may diverge from a non-eager
		// algorithm.
		if len(l1Blocks) < 2 {
			log.Info("eager batch wants to advance epoch, but could not without more L1 blocks", "current_epoch", epoch.ID())
			return BatchUndecided
		}
		batchOrigin = l1Blocks[1]
	} else {
		log.Warn("batch is for future epoch too far ahead, while it has the next timestamp, so it must be invalid", "current_epoch", epoch.ID())
		return BatchDrop
	}

	if batch.EpochHash != batchOrigin.Hash {
		log.Warn("batch is for different L1 chain, epoch hash does not match", "expected", batchOrigin.ID())
		return BatchDrop
	}

	if batch.Timestamp < batchOrigin.Time {
		log.Warn("batch timestamp is less than L1 origin timestamp", "l2_timestamp", batch.Timestamp, "l1_timestamp", batchOrigin.Time, "origin", batchOrigin.ID())
		return BatchDrop
	}

	// Check if we ran out of sequencer time drift
	if max := batchOrigin.Time + cfg.MaxSequencerDrift; batch.Timestamp > max {
		if len(batch.Transactions) == 0 {
			// If the sequencer is co-operating by producing an empty batch,
			// then allow the batch if it was the right thing to do to maintain the L2 time >= L1 time invariant.
			// We only check batches that do not advance the epoch, to ensure epoch advancement regardless of time drift is allowed.
			if epoch.Number == batchOrigin.Number {
				if len(l1Blocks) < 2 {
					log.Info("without the next L1 origin we cannot determine yet if this empty batch that exceeds the time drift is still valid")
					return BatchUndecided
				}
				nextOrigin := l1Blocks[1]
				if batch.Timestamp >= nextOrigin.Time { // check if the next L1 origin could have been adopted
					log.Info("batch exceeded sequencer time drift without adopting next origin, and next L1 origin would have been valid")
					return BatchDrop
				} else {
					log.Info("continuing with empty batch before late L1 block to preserve L2 time invariant")
				}
			}
		} else {
			// If the sequencer is ignoring the time drift rule, then drop the batch and force an empty batch instead,
			// as the sequencer is not allowed to include anything past this point without moving to the next epoch.
			log.Warn("batch exceeded sequencer time drift, sequencer must adopt new L1 origin to include transactions again", "max_time", max)
			return BatchDrop
		}
	}

	// We can do this check earlier, but it's a more intensive one, so we do this last.
	for i, txBytes := range batch.Transactions {
		if len(txBytes) == 0 {
			log.Warn("transaction data must not be empty, but found empty tx", "tx_index", i)
			return BatchDrop
		}
		if txBytes[0] == types.DepositTxType {
			log.Warn("sequencers may not embed any deposits into batch data, but found tx that has one", "tx_index", i)
			return BatchDrop
		}
	}

	return BatchAccept
}

func CheckSpanBatch(cfg *rollup.Config, log log.Logger, l1Blocks []eth.L1BlockRef, l2SafeHead eth.L2BlockRef, batch *SpanBatch, l1InclusionBlock eth.L1BlockRef) BatchValidity {
	log = batch.GetLogContext(log)

	// sanity check we have consistent inputs
	if len(l1Blocks) == 0 {
		log.Warn("missing L1 block input, cannot proceed with batch checking")
		return BatchUndecided
	}
	epoch := l1Blocks[0]

	nextTimestamp := l2SafeHead.Time + cfg.BlockTime

	if batch.batchTimestamp > nextTimestamp {
		log.Trace("received out-of-order batch for future processing after next batch", "next_timestamp", nextTimestamp)
		return BatchFuture
	}
	if batch.batchTimestamp < nextTimestamp {
		log.Warn("dropping batch with old timestamp", "min_timestamp", nextTimestamp)
		return BatchDrop
	}

	// dependent on above timestamp check. If the timestamp is correct, then it must build on top of the safe head.
	if !batch.CheckParentHash(l2SafeHead.Hash) {
		log.Warn("ignoring batch with mismatching parent hash", "current_safe_head", l2SafeHead.Hash)
		return BatchDrop
	}

	start_epoch_num := batch.blockOriginNums[0]

	// Filter out batches that were included too late.
	if start_epoch_num+cfg.SeqWindowSize < l1InclusionBlock.Number {
		log.Warn("batch was included too late, sequence window expired")
		return BatchDrop
	}

	// Check the L1 origin of the batch
	if start_epoch_num > epoch.Number+1 {
		log.Warn("batch is for future epoch too far ahead, while it has the next timestamp, so it must be invalid", "current_epoch", epoch.ID())
		return BatchDrop
	}

	end_epoch_num := batch.blockOriginNums[len(batch.blockOriginNums)-1]
	if end_epoch_num >= l1InclusionBlock.Number {
		log.Warn("the end of the span batch cannot past the L1 block it was included in")
		return BatchDrop
	}

	originChecked := false
	for _, l1Block := range l1Blocks {
		if l1Block.Number == end_epoch_num {
			if !batch.CheckOriginHash(l1Block.Hash) {
				log.Warn("batch is for different L1 chain, epoch hash does not match", "expected", l1Block.Hash)
				return BatchDrop
			}
			originChecked = true
		}
	}
	if !originChecked {
		log.Info("need more l1 blocks to check entire origins of span batch")
		return BatchUndecided
	}

	if start_epoch_num < epoch.Number {
		log.Warn("dropped batch, epoch is too old", "minimum", epoch.ID())
		return BatchDrop
	}

	originIdx := 0
	originAdvanced := false
	if start_epoch_num == epoch.Number+1 {
		originAdvanced = true
	}

	for i := uint64(0); i < batch.blockCount; i++ {
		if i > 0 {
			originAdvanced = false
			if batch.blockOriginNums[i] > batch.blockOriginNums[i-1] {
				originAdvanced = true
			}
		}
		if originAdvanced {
			originIdx += 1
		}
		l1Origin := l1Blocks[originIdx]
		blockTimestamp := batch.blockTimestamps[i]
		if blockTimestamp < l1Origin.Time {
			log.Warn("block timestamp is less than L1 origin timestamp", "l2_timestamp", blockTimestamp, "l1_timestamp", l1Origin.Time, "origin", l1Origin.ID())
			return BatchDrop
		}

		// Check if we ran out of sequencer time drift
		if max := l1Origin.Time + cfg.MaxSequencerDrift; blockTimestamp > max {
			if batch.blockTxCounts[i] == 0 {
				// If the sequencer is co-operating by producing an empty batch,
				// then allow the batch if it was the right thing to do to maintain the L2 time >= L1 time invariant.
				// We only check batches that do not advance the epoch, to ensure epoch advancement regardless of time drift is allowed.
				if !originAdvanced {
					if originIdx+1 >= len(l1Blocks) {
						log.Info("without the next L1 origin we cannot determine yet if this empty batch that exceeds the time drift is still valid")
						return BatchUndecided
					}
					if blockTimestamp >= l1Blocks[originIdx+1].Time { // check if the next L1 origin could have been adopted
						log.Info("batch exceeded sequencer time drift without adopting next origin, and next L1 origin would have been valid")
						return BatchDrop
					} else {
						log.Info("continuing with empty batch before late L1 block to preserve L2 time invariant")
					}
				}
			} else {
				// If the sequencer is ignoring the time drift rule, then drop the batch and force an empty batch instead,
				// as the sequencer is not allowed to include anything past this point without moving to the next epoch.
				log.Warn("batch exceeded sequencer time drift, sequencer must adopt new L1 origin to include transactions again", "max_time", max)
				return BatchDrop
			}
		}
	}

	// We can do this check earlier, but it's a more intensive one, so we do this last.
	for i, txData := range batch.txs.txDatas {
		if len(txData) == 0 {
			log.Warn("transaction data must not be empty, but found empty tx", "tx_index", i)
			return BatchDrop
		}
		if txData[0] == types.DepositTxType {
			log.Warn("sequencers may not embed any deposits into batch data, but found tx that has one", "tx_index", i)
			return BatchDrop
		}
	}

	return BatchAccept
}

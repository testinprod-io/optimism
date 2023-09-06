package derive

import (
	"bytes"
	"context"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"io"

	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-service/eth"
)

// ChannelInReader reads a batch from the channel
// This does decompression and limits the max RLP size
// This is a pure function from the channel, but each channel (or channel fragment)
// must be tagged with an L1 inclusion block to be passed to the batch queue.
type ChannelInReader struct {
	log log.Logger
	cfg *rollup.Config

	nextBatchFn func() (*BatchData, error)

	prev *ChannelBank

	metrics Metrics
}

var _ ResettableStage = (*ChannelInReader)(nil)

// NewChannelInReader creates a ChannelInReader, which should be Reset(origin) before use.
func NewChannelInReader(log log.Logger, cfg *rollup.Config, prev *ChannelBank, metrics Metrics) *ChannelInReader {
	return &ChannelInReader{
		log:     log,
		cfg:     cfg,
		prev:    prev,
		metrics: metrics,
	}
}

func (cr *ChannelInReader) Origin() eth.L1BlockRef {
	return cr.prev.Origin()
}

// TODO: Take full channel for better logging
func (cr *ChannelInReader) WriteChannel(data []byte) error {
	if f, err := BatchReader(bytes.NewBuffer(data)); err == nil {
		cr.nextBatchFn = f
		cr.metrics.RecordChannelInputBytes(len(data))
		return nil
	} else {
		cr.log.Error("Error creating batch reader from channel data", "err", err)
		return err
	}
}

// NextChannel forces the next read to continue with the next channel,
// resetting any decoding/decompression state to a fresh start.
func (cr *ChannelInReader) NextChannel() {
	cr.nextBatchFn = nil
}

// NextBatch pulls out the next batch from the channel if it has it.
// It returns io.EOF when it cannot make any more progress.
// It will return a temporary error if it needs to be called again to advance some internal state.
func (cr *ChannelInReader) NextBatch(ctx context.Context) (Batch, error) {
	if cr.nextBatchFn == nil {
		if data, err := cr.prev.NextData(ctx); err == io.EOF {
			return nil, io.EOF
		} else if err != nil {
			return nil, err
		} else {
			if err := cr.WriteChannel(data); err != nil {
				return nil, NewTemporaryError(err)
			}
		}
	}

	// TODO: can batch be non nil while err == io.EOF
	// This depends on the behavior of rlp.Stream
	batchData, err := cr.nextBatchFn()
	if err == io.EOF {
		cr.NextChannel()
		return nil, NotEnoughData
	} else if err != nil {
		cr.log.Warn("failed to read batch from channel reader, skipping to next channel now", "err", err)
		cr.NextChannel()
		return nil, NotEnoughData
	}
	if batchData.BatchType == SingularBatchType {
		return &batchData.SingularBatch, nil
	} else {
		spanBatch, err := batchData.RawSpanBatch.derive(cr.cfg.BlockTime, cr.cfg.Genesis.L2Time, cr.cfg.L2ChainID)
		if err != nil {
			return nil, err
		}
		return spanBatch, nil
	}
}

func (cr *ChannelInReader) Reset(ctx context.Context, _ eth.L1BlockRef, _ eth.SystemConfig) error {
	cr.nextBatchFn = nil
	return io.EOF
}

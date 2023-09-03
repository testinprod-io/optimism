package derive

import (
	"context"
	"encoding/binary"
	"github.com/ethereum/go-ethereum/core/types"
	"io"
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/require"

	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/testlog"
	"github.com/ethereum-optimism/optimism/op-node/testutils"
)

var minTs = uint64(0)
var maxTs = uint64(math.MaxUint64)
var genesisTimestamp = uint64(10)

type fakeBatchQueueInput struct {
	i       int
	batches []Batch
	errors  []error
	origin  eth.L1BlockRef
}

func (f *fakeBatchQueueInput) Origin() eth.L1BlockRef {
	return f.origin
}

func (f *fakeBatchQueueInput) NextBatch(ctx context.Context) (Batch, error) {
	if f.i >= len(f.batches) {
		return nil, io.EOF
	}
	b := f.batches[f.i]
	e := f.errors[f.i]
	f.i += 1
	return b, e
}

func mockHash(time uint64, layer uint8) common.Hash {
	hash := common.Hash{31: layer} // indicate L1 or L2
	binary.LittleEndian.PutUint64(hash[:], time)
	return hash
}

func b(chainId *big.Int, timestamp uint64, epoch eth.L1BlockRef) *SingularBatch {
	rng := rand.New(rand.NewSource(int64(timestamp)))
	signer := types.NewLondonSigner(chainId)
	tx := testutils.RandomTx(rng, new(big.Int).SetUint64(rng.Uint64()), signer)
	txData, _ := tx.MarshalBinary()
	return &SingularBatch{
		ParentHash:   mockHash(timestamp-2, 2),
		Timestamp:    timestamp,
		EpochNum:     rollup.Epoch(epoch.Number),
		EpochHash:    epoch.Hash,
		Transactions: []hexutil.Bytes{txData},
	}
}

func buildSpanBatches(t *testing.T, parent *eth.L2BlockRef, singularBatches []*SingularBatch, blockCounts []int, chainId *big.Int) []Batch {
	var spanBatches []Batch
	idx := 0
	for i, count := range blockCounts {
		var span SpanBatch
		originChangedBit := 0
		var parentOriginNum rollup.Epoch
		if i == 0 {
			parentOriginNum = rollup.Epoch(parent.L1Origin.Number)
		} else {
			parentOriginNum = singularBatches[idx-1].EpochNum
		}
		if parentOriginNum != singularBatches[idx].EpochNum {
			originChangedBit = 1
		}
		err := span.MergeSingularBatches(singularBatches[idx:idx+count], uint(originChangedBit), genesisTimestamp, chainId)
		require.NoError(t, err)
		spanBatches = append(spanBatches, &span)
		idx += count
	}
	return spanBatches
}

func getSpanBatchTime(batchType int) *uint64 {
	if batchType == SpanBatchType {
		return &minTs
	}
	return &maxTs
}

func L1Chain(l1Times []uint64) []eth.L1BlockRef {
	var out []eth.L1BlockRef
	var parentHash common.Hash
	for i, time := range l1Times {
		hash := mockHash(time, 1)
		out = append(out, eth.L1BlockRef{
			Hash:       hash,
			Number:     uint64(i),
			ParentHash: parentHash,
			Time:       time,
		})
		parentHash = hash
	}
	return out
}

func TestBatchQueue(t *testing.T) {
	tests := []struct {
		name string
		f    func(t *testing.T, batchType int)
	}{
		{"BatchQueueNewOrigin", BatchQueueNewOrigin},
		{"BatchQueueEager", BatchQueueEager},
		{"BatchQueueInvalidInternalAdvance", BatchQueueInvalidInternalAdvance},
		{"BatchQueueMissing", BatchQueueMissing},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name+"_SingularBatch", func(t *testing.T) {
			test.f(t, SingularBatchType)
		})
	}

	for _, test := range tests {
		test := test
		t.Run(test.name+"_SpanBatch", func(t *testing.T) {
			test.f(t, SpanBatchType)
		})
	}
}

// BatchQueueNewOrigin tests that the batch queue properly saves the new origin
// when the safehead's origin is ahead of the pipeline's origin (as is after a reset).
// This issue was fixed in https://github.com/ethereum-optimism/optimism/pull/3694
func BatchQueueNewOrigin(t *testing.T, batchType int) {
	log := testlog.Logger(t, log.LvlCrit)
	l1 := L1Chain([]uint64{10, 15, 20, 25})
	safeHead := eth.L2BlockRef{
		Hash:           mockHash(10, 2),
		Number:         0,
		ParentHash:     common.Hash{},
		Time:           20,
		L1Origin:       l1[2].ID(),
		SequenceNumber: 0,
	}
	cfg := &rollup.Config{
		Genesis: rollup.Genesis{
			L2Time: genesisTimestamp,
		},
		BlockTime:         2,
		MaxSequencerDrift: 600,
		SeqWindowSize:     2,
		SpanBatchTime:     getSpanBatchTime(batchType),
	}

	input := &fakeBatchQueueInput{
		batches: []Batch{nil},
		errors:  []error{io.EOF},
		origin:  l1[0],
	}

	bq := NewBatchQueue(log, cfg, input)
	_ = bq.Reset(context.Background(), l1[0], eth.SystemConfig{})
	require.Equal(t, []eth.L1BlockRef{l1[0]}, bq.l1Blocks)

	// Prev Origin: 0; Safehead Origin: 2; Internal Origin: 0
	// Should return no data but keep the same origin
	data, err := bq.NextBatch(context.Background(), safeHead)
	require.Nil(t, data)
	require.Equal(t, io.EOF, err)
	require.Equal(t, []eth.L1BlockRef{l1[0]}, bq.l1Blocks)
	require.Equal(t, l1[0], bq.origin)

	// Prev Origin: 1; Safehead Origin: 2; Internal Origin: 0
	// Should wipe l1blocks + advance internal origin
	input.origin = l1[1]
	data, err = bq.NextBatch(context.Background(), safeHead)
	require.Nil(t, data)
	require.Equal(t, io.EOF, err)
	require.Empty(t, bq.l1Blocks)
	require.Equal(t, l1[1], bq.origin)

	// Prev Origin: 2; Safehead Origin: 2; Internal Origin: 1
	// Should add to l1Blocks + advance internal origin
	input.origin = l1[2]
	data, err = bq.NextBatch(context.Background(), safeHead)
	require.Nil(t, data)
	require.Equal(t, io.EOF, err)
	require.Equal(t, []eth.L1BlockRef{l1[2]}, bq.l1Blocks)
	require.Equal(t, l1[2], bq.origin)
}

// BatchQueueEager adds a bunch of contiguous batches and asserts that
// enough calls to `NextBatch` return all of those batches.
func BatchQueueEager(t *testing.T, batchType int) {
	log := testlog.Logger(t, log.LvlCrit)
	l1 := L1Chain([]uint64{10, 20, 30})
	chainId := big.NewInt(1234)
	safeHead := eth.L2BlockRef{
		Hash:           mockHash(10, 2),
		Number:         0,
		ParentHash:     common.Hash{},
		Time:           10,
		L1Origin:       l1[0].ID(),
		SequenceNumber: 0,
	}
	cfg := &rollup.Config{
		Genesis: rollup.Genesis{
			L2Time: genesisTimestamp,
		},
		BlockTime:         2,
		MaxSequencerDrift: 600,
		SeqWindowSize:     30,
		SpanBatchTime:     getSpanBatchTime(batchType),
		L2ChainID:         chainId,
	}

	singularBatches := []*SingularBatch{b(cfg.L2ChainID, 12, l1[0]), b(cfg.L2ChainID, 14, l1[0]), b(cfg.L2ChainID, 16, l1[0]), b(cfg.L2ChainID, 18, l1[0]), b(cfg.L2ChainID, 20, l1[0]), b(cfg.L2ChainID, 22, l1[0]), nil}
	errors := []error{nil, nil, nil, nil, nil, nil, io.EOF}
	inputErrors := errors
	var batches []Batch
	if batchType == SpanBatchType {
		spanBlockCounts := []int{1, 2, 3}
		inputErrors = []error{nil, nil, nil, io.EOF}
		batches = buildSpanBatches(t, &safeHead, singularBatches, spanBlockCounts, chainId)
		batches = append(batches, nil)
	} else {
		for _, singularBatch := range singularBatches {
			batches = append(batches, singularBatch)
		}
	}

	input := &fakeBatchQueueInput{
		batches: batches,
		errors:  inputErrors,
		origin:  l1[0],
	}

	bq := NewBatchQueue(log, cfg, input)
	_ = bq.Reset(context.Background(), l1[0], eth.SystemConfig{})
	// Advance the origin
	input.origin = l1[1]

	for i := 0; i < len(singularBatches); i++ {
		b, e := bq.NextBatch(context.Background(), safeHead)
		require.ErrorIs(t, e, errors[i])
		if b == nil {
			require.Nil(t, singularBatches[i])
		} else {
			require.Equal(t, singularBatches[i], b)
			safeHead.Number += 1
			safeHead.Time += 2
			safeHead.Hash = mockHash(b.Timestamp, 2)
			safeHead.L1Origin = b.Epoch()
		}
	}
}

// BatchQueueInvalidInternalAdvance asserts that we do not miss an epoch when generating batches.
// This is a regression test for CLI-3378.
func BatchQueueInvalidInternalAdvance(t *testing.T, batchType int) {
	log := testlog.Logger(t, log.LvlTrace)
	l1 := L1Chain([]uint64{10, 15, 20, 25, 30})
	chainId := big.NewInt(1234)
	safeHead := eth.L2BlockRef{
		Hash:           mockHash(10, 2),
		Number:         0,
		ParentHash:     common.Hash{},
		Time:           10,
		L1Origin:       l1[0].ID(),
		SequenceNumber: 0,
	}
	cfg := &rollup.Config{
		Genesis: rollup.Genesis{
			L2Time: genesisTimestamp,
		},
		BlockTime:         2,
		MaxSequencerDrift: 600,
		SeqWindowSize:     2,
		SpanBatchTime:     getSpanBatchTime(batchType),
		L2ChainID:         chainId,
	}

	singularBatches := []*SingularBatch{b(cfg.L2ChainID, 12, l1[0]), b(cfg.L2ChainID, 14, l1[0]), b(cfg.L2ChainID, 16, l1[0]), b(cfg.L2ChainID, 18, l1[0]), b(cfg.L2ChainID, 20, l1[0]), b(cfg.L2ChainID, 22, l1[0]), nil}
	errors := []error{nil, nil, nil, nil, nil, nil, io.EOF}
	inputErrors := errors
	var batches []Batch
	if batchType == SpanBatchType {
		spanBlockCounts := []int{1, 2, 3}
		inputErrors = []error{nil, nil, nil, io.EOF}
		batches = buildSpanBatches(t, &safeHead, singularBatches, spanBlockCounts, chainId)
		batches = append(batches, nil)
	} else {
		for _, singularBatch := range singularBatches {
			batches = append(batches, singularBatch)
		}
	}

	input := &fakeBatchQueueInput{
		batches: batches,
		errors:  inputErrors,
		origin:  l1[0],
	}

	bq := NewBatchQueue(log, cfg, input)
	_ = bq.Reset(context.Background(), l1[0], eth.SystemConfig{})

	// Load continuous batches for epoch 0
	for i := 0; i < len(singularBatches); i++ {
		b, e := bq.NextBatch(context.Background(), safeHead)
		require.ErrorIs(t, e, errors[i])
		if b == nil {
			require.Nil(t, singularBatches[i])
		} else {
			require.Equal(t, singularBatches[i], b)
			safeHead.Number += 1
			safeHead.Time += 2
			safeHead.Hash = mockHash(b.Timestamp, 2)
			safeHead.L1Origin = b.Epoch()
		}
	}

	// Advance to origin 1. No forced batches yet.
	input.origin = l1[1]
	b, e := bq.NextBatch(context.Background(), safeHead)
	require.ErrorIs(t, e, io.EOF)
	require.Nil(t, b)

	// Advance to origin 2. No forced batches yet because we are still on epoch 0
	// & have batches for epoch 0.
	input.origin = l1[2]
	b, e = bq.NextBatch(context.Background(), safeHead)
	require.ErrorIs(t, e, io.EOF)
	require.Nil(t, b)

	// Advance to origin 3. Should generate one empty batch.
	input.origin = l1[3]
	b, e = bq.NextBatch(context.Background(), safeHead)
	require.Nil(t, e)
	require.NotNil(t, b)
	require.Equal(t, safeHead.Time+2, b.Timestamp)
	require.Equal(t, rollup.Epoch(1), b.EpochNum)
	safeHead.Number += 1
	safeHead.Time += 2
	safeHead.Hash = mockHash(b.Timestamp, 2)
	safeHead.L1Origin = b.Epoch()
	b, e = bq.NextBatch(context.Background(), safeHead)
	require.ErrorIs(t, e, io.EOF)
	require.Nil(t, b)

	// Advance to origin 4. Should generate one empty batch.
	input.origin = l1[4]
	b, e = bq.NextBatch(context.Background(), safeHead)
	require.Nil(t, e)
	require.NotNil(t, b)
	require.Equal(t, rollup.Epoch(2), b.EpochNum)
	require.Equal(t, safeHead.Time+2, b.Timestamp)
	safeHead.Number += 1
	safeHead.Time += 2
	safeHead.Hash = mockHash(b.Timestamp, 2)
	safeHead.L1Origin = b.Epoch()
	b, e = bq.NextBatch(context.Background(), safeHead)
	require.ErrorIs(t, e, io.EOF)
	require.Nil(t, b)

}

func BatchQueueMissing(t *testing.T, batchType int) {
	log := testlog.Logger(t, log.LvlCrit)
	l1 := L1Chain([]uint64{10, 15, 20, 25})
	chainId := big.NewInt(1234)
	safeHead := eth.L2BlockRef{
		Hash:           mockHash(10, 2),
		Number:         0,
		ParentHash:     common.Hash{},
		Time:           10,
		L1Origin:       l1[0].ID(),
		SequenceNumber: 0,
	}
	cfg := &rollup.Config{
		Genesis: rollup.Genesis{
			L2Time: genesisTimestamp,
		},
		BlockTime:         2,
		MaxSequencerDrift: 600,
		SeqWindowSize:     2,
		SpanBatchTime:     getSpanBatchTime(batchType),
		L2ChainID:         chainId,
	}

	// The batches at 18 and 20 are skipped to stop 22 from being eagerly processed.
	// This test checks that batch timestamp 12 & 14 are created, 16 is used, and 18 is advancing the epoch.
	// Due to the large sequencer time drift 16 is perfectly valid to have epoch 0 as origin.
	singularBatches := []*SingularBatch{b(cfg.L2ChainID, 16, l1[0]), b(cfg.L2ChainID, 22, l1[1])}
	inputErrors := []error{nil, nil}
	var batches []Batch
	if batchType == SpanBatchType {
		spanBlockCounts := []int{1, 1}
		inputErrors = []error{nil, nil, nil, io.EOF}
		batches = buildSpanBatches(t, &safeHead, singularBatches, spanBlockCounts, chainId)
	} else {
		for _, singularBatch := range singularBatches {
			batches = append(batches, singularBatch)
		}
	}

	input := &fakeBatchQueueInput{
		batches: batches,
		errors:  inputErrors,
		origin:  l1[0],
	}

	bq := NewBatchQueue(log, cfg, input)
	_ = bq.Reset(context.Background(), l1[0], eth.SystemConfig{})

	for i := 0; i < len(singularBatches); i++ {
		b, e := bq.NextBatch(context.Background(), safeHead)
		require.ErrorIs(t, e, NotEnoughData)
		require.Nil(t, b)
	}

	// advance origin. Underlying stage still has no more batches
	// This is not enough to auto advance yet
	input.origin = l1[1]
	b, e := bq.NextBatch(context.Background(), safeHead)
	require.ErrorIs(t, e, io.EOF)
	require.Nil(t, b)

	// Advance the origin. At this point batch timestamps 12 and 14 will be created
	input.origin = l1[2]

	// Check for a generated batch at t = 12
	b, e = bq.NextBatch(context.Background(), safeHead)
	require.Nil(t, e)
	require.Equal(t, b.Timestamp, uint64(12))
	require.Empty(t, b.Transactions)
	require.Equal(t, rollup.Epoch(0), b.EpochNum)
	safeHead.Number += 1
	safeHead.Time += 2
	safeHead.Hash = mockHash(b.Timestamp, 2)

	// Check for generated batch at t = 14
	b, e = bq.NextBatch(context.Background(), safeHead)
	require.Nil(t, e)
	require.Equal(t, b.Timestamp, uint64(14))
	require.Empty(t, b.Transactions)
	require.Equal(t, rollup.Epoch(0), b.EpochNum)
	safeHead.Number += 1
	safeHead.Time += 2
	safeHead.Hash = mockHash(b.Timestamp, 2)

	// Check for the inputted batch at t = 16
	b, e = bq.NextBatch(context.Background(), safeHead)
	require.Nil(t, e)
	require.Equal(t, b, singularBatches[0])
	require.Equal(t, rollup.Epoch(0), b.EpochNum)
	safeHead.Number += 1
	safeHead.Time += 2
	safeHead.Hash = mockHash(b.Timestamp, 2)

	// Advance the origin. At this point the batch with timestamp 18 will be created
	input.origin = l1[3]

	// Check for the generated batch at t = 18. This batch advances the epoch
	// Note: We need one io.EOF returned from the bq that advances the internal L1 Blocks view
	// before the batch will be auto generated
	_, e = bq.NextBatch(context.Background(), safeHead)
	require.Equal(t, e, io.EOF)
	b, e = bq.NextBatch(context.Background(), safeHead)
	require.Nil(t, e)
	require.Equal(t, b.Timestamp, uint64(18))
	require.Empty(t, b.Transactions)
	require.Equal(t, rollup.Epoch(1), b.EpochNum)
}

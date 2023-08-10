package batcher

import (
	"io"
	"math"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/ethereum-optimism/optimism/op-batcher/compressor"
	"github.com/ethereum-optimism/optimism/op-batcher/metrics"
	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	derivetest "github.com/ethereum-optimism/optimism/op-node/rollup/derive/test"
	"github.com/ethereum-optimism/optimism/op-node/testlog"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/require"
)

func TestChannelManagerBatchType(t *testing.T) {
	maxTs := uint64(math.MaxUint64)
	spanBatchNotActivated := rollup.Config{
		SpanBatchTime: &maxTs,
	}
	zeroTs := uint64(0)
	spanBatchActivated := rollup.Config{
		SpanBatchTime: &zeroTs,
	}
	safeHead := eth.L2BlockRef{
		Time: 0,
	}
	tests := []struct {
		name string
		f    func(t *testing.T, rcfg *rollup.Config, safeHead *eth.L2BlockRef)
	}{
		{"ChannelManagerReturnsErrReorg", ChannelManagerReturnsErrReorg},
		{"ChannelManagerReturnsErrReorgWhenDrained", ChannelManagerReturnsErrReorgWhenDrained},
		{"ChannelManager_Clear", ChannelManager_Clear},
		{"ChannelManager_TxResend", ChannelManager_TxResend},
		{"ChannelManagerCloseBeforeFirstUse", ChannelManagerCloseBeforeFirstUse},
		{"ChannelManagerCloseNoPendingChannel", ChannelManagerCloseNoPendingChannel},
		{"ChannelManagerClosePendingChannel", ChannelManagerClosePendingChannel},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name+"_BatchV1", func(t *testing.T) {
			test.f(t, &spanBatchNotActivated, &safeHead)
		})
	}

	for _, test := range tests {
		test := test
		t.Run(test.name+"_BatchV2", func(t *testing.T) {
			test.f(t, &spanBatchActivated, &safeHead)
		})
	}
}

// ChannelManagerReturnsErrReorg ensures that the channel manager
// detects a reorg when it has cached L1 blocks.
func ChannelManagerReturnsErrReorg(t *testing.T, rcfg *rollup.Config, safeHead *eth.L2BlockRef) {
	log := testlog.Logger(t, log.LvlCrit)
	m := NewChannelManager(log, metrics.NoopMetrics, ChannelConfig{}, rcfg, safeHead)

	a := types.NewBlock(&types.Header{
		Number: big.NewInt(0),
	}, nil, nil, nil, nil)
	b := types.NewBlock(&types.Header{
		Number:     big.NewInt(1),
		ParentHash: a.Hash(),
	}, nil, nil, nil, nil)
	c := types.NewBlock(&types.Header{
		Number:     big.NewInt(2),
		ParentHash: b.Hash(),
	}, nil, nil, nil, nil)
	x := types.NewBlock(&types.Header{
		Number:     big.NewInt(2),
		ParentHash: common.Hash{0xff},
	}, nil, nil, nil, nil)

	require.NoError(t, m.AddL2Block(a))
	require.NoError(t, m.AddL2Block(b))
	require.NoError(t, m.AddL2Block(c))
	require.ErrorIs(t, m.AddL2Block(x), ErrReorg)

	require.Equal(t, []*types.Block{a, b, c}, m.blocks)
}

// ChannelManagerReturnsErrReorgWhenDrained ensures that the channel manager
// detects a reorg even if it does not have any blocks inside it.
func ChannelManagerReturnsErrReorgWhenDrained(t *testing.T, rcfg *rollup.Config, safeHead *eth.L2BlockRef) {
	log := testlog.Logger(t, log.LvlCrit)
	m := NewChannelManager(log, metrics.NoopMetrics,
		ChannelConfig{
			MaxFrameSize: 120_000,
			CompressorConfig: compressor.Config{
				TargetFrameSize:  1,
				TargetNumFrames:  1,
				ApproxComprRatio: 1.0,
			},
		},
		rcfg, safeHead,
	)

	a := newMiniL2Block(0)
	x := newMiniL2BlockWithNumberParent(0, big.NewInt(1), common.Hash{0xff})

	require.NoError(t, m.AddL2Block(a))

	_, err := m.TxData(eth.BlockID{})
	require.NoError(t, err)
	_, err = m.TxData(eth.BlockID{})
	require.ErrorIs(t, err, io.EOF)

	require.ErrorIs(t, m.AddL2Block(x), ErrReorg)
}

// ChannelManager_Clear tests clearing the channel manager.
func ChannelManager_Clear(t *testing.T, rcfg *rollup.Config, safeHead *eth.L2BlockRef) {
	require := require.New(t)

	// Create a channel manager
	log := testlog.Logger(t, log.LvlCrit)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	m := NewChannelManager(log, metrics.NoopMetrics, ChannelConfig{
		// Need to set the channel timeout here so we don't clear pending
		// channels on confirmation. This would result in [TxConfirmed]
		// clearing confirmed transactions, and reseting the pendingChannels map
		ChannelTimeout: 10,
		// Have to set the max frame size here otherwise the channel builder would not
		// be able to output any frames
		MaxFrameSize: 24,
		CompressorConfig: compressor.Config{
			TargetFrameSize:  24,
			TargetNumFrames:  1,
			ApproxComprRatio: 1.0,
		},
	},
		rcfg, safeHead,
	)

	// Channel Manager state should be empty by default
	require.Empty(m.blocks)
	require.Equal(common.Hash{}, m.tip)
	require.Nil(m.currentChannel)
	require.Empty(m.channelQueue)
	require.Empty(m.txChannels)

	// Add a block to the channel manager
	a, _ := derivetest.RandomL2Block(rng, 4)
	newL1Tip := a.Hash()
	l1BlockID := eth.BlockID{
		Hash:   a.Hash(),
		Number: a.NumberU64(),
	}
	require.NoError(m.AddL2Block(a))

	// Make sure there is a channel
	require.NoError(m.ensureChannelWithSpace(l1BlockID))
	require.NotNil(m.currentChannel)
	require.Len(m.currentChannel.confirmedTransactions, 0)

	// Process the blocks
	// We should have a pending channel with 1 frame
	// and no more blocks since processBlocks consumes
	// the list
	require.NoError(m.processBlocks())
	require.NoError(m.currentChannel.channelBuilder.co.Flush())
	require.NoError(m.currentChannel.OutputFrames())
	_, err := m.nextTxData(m.currentChannel)
	require.NoError(err)
	require.Len(m.blocks, 0)
	require.Equal(newL1Tip, m.tip)
	require.Len(m.currentChannel.pendingTransactions, 1)

	// Add a new block so we can test clearing
	// the channel manager with a full state
	b := types.NewBlock(&types.Header{
		Number:     big.NewInt(1),
		ParentHash: a.Hash(),
	}, nil, nil, nil, nil)
	require.NoError(m.AddL2Block(b))
	require.Len(m.blocks, 1)
	require.Equal(b.Hash(), m.tip)

	// Clear the channel manager
	m.Clear(&eth.L2BlockRef{})

	// Check that the entire channel manager state cleared
	require.Empty(m.blocks)
	require.Equal(common.Hash{}, m.tip)
	require.Nil(m.currentChannel)
	require.Empty(m.channelQueue)
	require.Empty(m.txChannels)
}

func ChannelManager_TxResend(t *testing.T, rcfg *rollup.Config, safeHead *eth.L2BlockRef) {
	require := require.New(t)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	log := testlog.Logger(t, log.LvlError)
	m := NewChannelManager(log, metrics.NoopMetrics,
		ChannelConfig{
			MaxFrameSize: 120_000,
			CompressorConfig: compressor.Config{
				TargetFrameSize:  1,
				TargetNumFrames:  1,
				ApproxComprRatio: 1.0,
			},
		},
		rcfg, safeHead,
	)

	a, _ := derivetest.RandomL2Block(rng, 4)

	require.NoError(m.AddL2Block(a))

	txdata0, err := m.TxData(eth.BlockID{})
	require.NoError(err)
	txdata0bytes := txdata0.Bytes()
	data0 := make([]byte, len(txdata0bytes))
	// make sure we have a clone for later comparison
	copy(data0, txdata0bytes)

	// ensure channel is drained
	_, err = m.TxData(eth.BlockID{})
	require.ErrorIs(err, io.EOF)

	// requeue frame
	m.TxFailed(txdata0.ID())

	txdata1, err := m.TxData(eth.BlockID{})
	require.NoError(err)

	data1 := txdata1.Bytes()
	require.Equal(data1, data0)
	fs, err := derive.ParseFrames(data1)
	require.NoError(err)
	require.Len(fs, 1)
}

// ChannelManagerCloseBeforeFirstUse ensures that the channel manager
// will not produce any frames if closed immediately.
func ChannelManagerCloseBeforeFirstUse(t *testing.T, rcfg *rollup.Config, safeHead *eth.L2BlockRef) {
	require := require.New(t)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	log := testlog.Logger(t, log.LvlCrit)
	m := NewChannelManager(log, metrics.NoopMetrics,
		ChannelConfig{
			MaxFrameSize:   100,
			ChannelTimeout: 1000,
			CompressorConfig: compressor.Config{
				TargetFrameSize:  0,
				ApproxComprRatio: 1.0,
			},
		},
		rcfg, safeHead,
	)

	a, _ := derivetest.RandomL2Block(rng, 4)

	m.Close()

	err := m.AddL2Block(a)
	require.NoError(err, "Failed to add L2 block")

	_, err = m.TxData(eth.BlockID{})
	require.ErrorIs(err, io.EOF, "Expected closed channel manager to contain no tx data")
}

// ChannelManagerCloseNoPendingChannel ensures that the channel manager
// can gracefully close with no pending channels, and will not emit any new
// channel frames.
func ChannelManagerCloseNoPendingChannel(t *testing.T, rcfg *rollup.Config, safeHead *eth.L2BlockRef) {
	require := require.New(t)
	log := testlog.Logger(t, log.LvlCrit)
	m := NewChannelManager(log, metrics.NoopMetrics,
		ChannelConfig{
			MaxFrameSize:   100,
			ChannelTimeout: 1000,
			CompressorConfig: compressor.Config{
				TargetFrameSize:  1,
				TargetNumFrames:  1,
				ApproxComprRatio: 1.0,
			},
		},
		rcfg, safeHead,
	)
	a := newMiniL2Block(0)
	b := newMiniL2BlockWithNumberParent(0, big.NewInt(1), a.Hash())

	err := m.AddL2Block(a)
	require.NoError(err, "Failed to add L2 block")

	txdata, err := m.TxData(eth.BlockID{})
	require.NoError(err, "Expected channel manager to return valid tx data")

	m.TxConfirmed(txdata.ID(), eth.BlockID{})

	_, err = m.TxData(eth.BlockID{})
	require.ErrorIs(err, io.EOF, "Expected channel manager to EOF")

	m.Close()

	err = m.AddL2Block(b)
	require.NoError(err, "Failed to add L2 block")

	_, err = m.TxData(eth.BlockID{})
	require.ErrorIs(err, io.EOF, "Expected closed channel manager to return no new tx data")
}

// ChannelManagerCloseNoPendingChannel ensures that the channel manager
// can gracefully close with a pending channel, and will not produce any
// new channel frames after this point.
func ChannelManagerClosePendingChannel(t *testing.T, rcfg *rollup.Config, safeHead *eth.L2BlockRef) {
	require := require.New(t)
	log := testlog.Logger(t, log.LvlCrit)
	m := NewChannelManager(log, metrics.NoopMetrics,
		ChannelConfig{
			MaxFrameSize:   1000,
			ChannelTimeout: 1000,
			CompressorConfig: compressor.Config{
				TargetNumFrames:  100,
				TargetFrameSize:  1000,
				ApproxComprRatio: 1.0,
			},
		},
		rcfg, safeHead,
	)

	a := newMiniL2Block(50_000)
	b := newMiniL2BlockWithNumberParent(10, big.NewInt(1), a.Hash())

	err := m.AddL2Block(a)
	require.NoError(err, "Failed to add L2 block")

	txdata, err := m.TxData(eth.BlockID{})
	require.NoError(err, "Expected channel manager to produce valid tx data")

	m.TxConfirmed(txdata.ID(), eth.BlockID{})

	m.Close()

	txdata, err = m.TxData(eth.BlockID{})
	require.NoError(err, "Expected channel manager to produce tx data from remaining L2 block data")

	m.TxConfirmed(txdata.ID(), eth.BlockID{})

	_, err = m.TxData(eth.BlockID{})
	require.ErrorIs(err, io.EOF, "Expected channel manager to have no more tx data")

	err = m.AddL2Block(b)
	require.NoError(err, "Failed to add L2 block")

	_, err = m.TxData(eth.BlockID{})
	require.ErrorIs(err, io.EOF, "Expected closed channel manager to produce no more tx data")
}

// ChannelManagerCloseAllTxsFailed ensures that the channel manager
// can gracefully close after producing transaction frames if none of these
// have successfully landed on chain.
func ChannelManagerCloseAllTxsFailed(t *testing.T, rcfg *rollup.Config, safeHead *eth.L2BlockRef) {
	require := require.New(t)
	log := testlog.Logger(t, log.LvlCrit)
	m := NewChannelManager(log, metrics.NoopMetrics,
		ChannelConfig{
			MaxFrameSize:   1000,
			ChannelTimeout: 1000,
			CompressorConfig: compressor.Config{
				TargetNumFrames:  100,
				TargetFrameSize:  1000,
				ApproxComprRatio: 1.0,
			},
		}, rcfg, safeHead,
	)

	a := newMiniL2Block(50_000)

	err := m.AddL2Block(a)
	require.NoError(err, "Failed to add L2 block")

	txdata, err := m.TxData(eth.BlockID{})
	require.NoError(err, "Expected channel manager to produce valid tx data")

	m.TxFailed(txdata.ID())

	// Show that this data will continue to be emitted as long as the transaction
	// fails and the channel manager is not closed
	txdata, err = m.TxData(eth.BlockID{})
	require.NoError(err, "Expected channel manager to re-attempt the failed transaction")

	m.TxFailed(txdata.ID())

	m.Close()

	_, err = m.TxData(eth.BlockID{})
	require.ErrorIs(err, io.EOF, "Expected closed channel manager to produce no more tx data")
}

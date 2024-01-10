package actions

import (
	"errors"
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum-optimism/optimism/op-batcher/compressor"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-node/rollup/sync"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/sources"
	"github.com/ethereum-optimism/optimism/op-service/testlog"
	"github.com/ethereum-optimism/optimism/op-service/testutils"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/require"
)

// TestSyncBatchType run each sync test case in singular batch mode and span batch mode.
func TestSyncBatchType(t *testing.T) {
	tests := []struct {
		name string
		f    func(gt *testing.T, deltaTimeOffset *hexutil.Uint64)
	}{
		{"DerivationWithFlakyL1RPC", DerivationWithFlakyL1RPC},
		{"FinalizeWhileSyncing", FinalizeWhileSyncing},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name+"_SingularBatch", func(t *testing.T) {
			test.f(t, nil)
		})
	}

	deltaTimeOffset := hexutil.Uint64(0)
	for _, test := range tests {
		test := test
		t.Run(test.name+"_SpanBatch", func(t *testing.T) {
			test.f(t, &deltaTimeOffset)
		})
	}
}

func DerivationWithFlakyL1RPC(gt *testing.T, deltaTimeOffset *hexutil.Uint64) {
	t := NewDefaultTesting(gt)
	dp := e2eutils.MakeDeployParams(t, defaultRollupTestParams)
	dp.DeployConfig.L2GenesisDeltaTimeOffset = deltaTimeOffset
	sd := e2eutils.Setup(t, dp, defaultAlloc)
	log := testlog.Logger(t, log.LvlError) // mute all the temporary derivation errors that we forcefully create
	_, _, miner, sequencer, _, verifier, _, batcher := setupReorgTestActors(t, dp, sd, log)

	rng := rand.New(rand.NewSource(1234))
	sequencer.ActL2PipelineFull(t)
	verifier.ActL2PipelineFull(t)

	// build a L1 chain with 20 blocks and matching L2 chain and batches to test some derivation work
	miner.ActEmptyBlock(t)
	for i := 0; i < 20; i++ {
		sequencer.ActL1HeadSignal(t)
		sequencer.ActL2PipelineFull(t)
		sequencer.ActBuildToL1Head(t)
		batcher.ActSubmitAll(t)
		miner.ActL1StartBlock(12)(t)
		miner.ActL1IncludeTx(batcher.batcherAddr)(t)
		miner.ActL1EndBlock(t)
	}
	// Make verifier aware of head
	verifier.ActL1HeadSignal(t)

	// Now make the L1 RPC very flaky: requests will randomly fail with 50% chance
	miner.MockL1RPCErrors(func() error {
		if rng.Intn(2) == 0 {
			return errors.New("mock rpc error")
		}
		return nil
	})

	// And sync the verifier
	verifier.ActL2PipelineFull(t)
	// Verifier should be synced, even though it hit lots of temporary L1 RPC errors
	require.Equal(t, sequencer.L2Unsafe(), verifier.L2Safe(), "verifier is synced")
}

func FinalizeWhileSyncing(gt *testing.T, deltaTimeOffset *hexutil.Uint64) {
	t := NewDefaultTesting(gt)
	dp := e2eutils.MakeDeployParams(t, defaultRollupTestParams)
	dp.DeployConfig.L2GenesisDeltaTimeOffset = deltaTimeOffset
	sd := e2eutils.Setup(t, dp, defaultAlloc)
	log := testlog.Logger(t, log.LvlError) // mute all the temporary derivation errors that we forcefully create
	_, _, miner, sequencer, _, verifier, _, batcher := setupReorgTestActors(t, dp, sd, log)

	sequencer.ActL2PipelineFull(t)
	verifier.ActL2PipelineFull(t)

	verifierStartStatus := verifier.SyncStatus()

	// Build an L1 chain with 64 + 1 blocks, containing batches of L2 chain.
	// Enough to go past the finalityDelay of the engine queue,
	// to make the verifier finalize while it syncs.
	miner.ActEmptyBlock(t)
	for i := 0; i < 64+1; i++ {
		sequencer.ActL1HeadSignal(t)
		sequencer.ActL2PipelineFull(t)
		sequencer.ActBuildToL1Head(t)
		batcher.ActSubmitAll(t)
		miner.ActL1StartBlock(12)(t)
		miner.ActL1IncludeTx(batcher.batcherAddr)(t)
		miner.ActL1EndBlock(t)
	}
	l1Head := miner.l1Chain.CurrentHeader()
	// finalize all of L1
	miner.ActL1Safe(t, l1Head.Number.Uint64())
	miner.ActL1Finalize(t, l1Head.Number.Uint64())

	// Now signal L1 finality to the verifier, while the verifier is not synced.
	verifier.ActL1HeadSignal(t)
	verifier.ActL1SafeSignal(t)
	verifier.ActL1FinalizedSignal(t)

	// Now sync the verifier, without repeating the signal.
	// While it's syncing, it should finalize on interval now, based on the future L1 finalized block it remembered.
	verifier.ActL2PipelineFull(t)

	// Verify the verifier finalized something new
	require.Less(t, verifierStartStatus.FinalizedL2.Number, verifier.SyncStatus().FinalizedL2.Number, "verifier finalized L2 blocks during sync")
}

// TestUnsafeSync tests that a verifier properly imports unsafe blocks via gossip.
func TestUnsafeSync(gt *testing.T) {
	t := NewDefaultTesting(gt)
	dp := e2eutils.MakeDeployParams(t, defaultRollupTestParams)
	sd := e2eutils.Setup(t, dp, defaultAlloc)
	log := testlog.Logger(t, log.LvlInfo)

	sd, _, _, sequencer, seqEng, verifier, _, _ := setupReorgTestActors(t, dp, sd, log)
	seqEngCl, err := sources.NewEngineClient(seqEng.RPCClient(), log, nil, sources.EngineClientDefaultConfig(sd.RollupCfg))
	require.NoError(t, err)

	sequencer.ActL2PipelineFull(t)
	verifier.ActL2PipelineFull(t)

	for i := 0; i < 10; i++ {
		// Build a L2 block
		sequencer.ActL2StartBlock(t)
		sequencer.ActL2EndBlock(t)
		// Notify new L2 block to verifier by unsafe gossip
		seqHead, err := seqEngCl.PayloadByLabel(t.Ctx(), eth.Unsafe)
		require.NoError(t, err)
		verifier.ActL2UnsafeGossipReceive(seqHead)(t)
		// Handle unsafe payload
		verifier.ActL2PipelineFull(t)
		// Verifier must advance its unsafe head.
		require.Equal(t, sequencer.L2Unsafe().Hash, verifier.L2Unsafe().Hash)
	}
}

func TestBackupUnsafe(gt *testing.T) {
	t := NewDefaultTesting(gt)
	dp := e2eutils.MakeDeployParams(t, defaultRollupTestParams)
	minTs := hexutil.Uint64(0)
	// Activate Delta hardfork
	dp.DeployConfig.L2GenesisDeltaTimeOffset = &minTs
	dp.DeployConfig.L2BlockTime = 2
	sd := e2eutils.Setup(t, dp, defaultAlloc)
	log := testlog.Logger(t, log.LvlInfo)
	_, dp, miner, sequencer, seqEng, verifier, _, batcher := setupReorgTestActors(t, dp, sd, log)
	l2Cl := seqEng.EthClient()
	seqEngCl, err := sources.NewEngineClient(seqEng.RPCClient(), log, nil, sources.EngineClientDefaultConfig(sd.RollupCfg))
	require.NoError(t, err)

	rng := rand.New(rand.NewSource(1234))
	signer := types.LatestSigner(sd.L2Cfg.Config)

	sequencer.ActL2PipelineFull(t)
	verifier.ActL2PipelineFull(t)

	// Create block A1 ~ A5
	for i := 0; i < 5; i++ {
		// Build a L2 block
		sequencer.ActL2StartBlock(t)
		sequencer.ActL2EndBlock(t)

		// Notify new L2 block to verifier by unsafe gossip
		seqHead, err := seqEngCl.PayloadByLabel(t.Ctx(), eth.Unsafe)
		require.NoError(t, err)
		verifier.ActL2UnsafeGossipReceive(seqHead)(t)
	}

	seqHead, err := seqEngCl.PayloadByLabel(t.Ctx(), eth.Unsafe)
	require.NoError(t, err)
	// eventually correct hash for A5
	targetUnsafeHeadHash := seqHead.BlockHash

	// only advance unsafe head to A5
	require.Equal(t, sequencer.L2Unsafe().Number, uint64(5))
	require.Equal(t, sequencer.L2Safe().Number, uint64(0))

	// Handle unsafe payload
	verifier.ActL2PipelineFull(t)
	// only advance unsafe head to A5
	require.Equal(t, verifier.L2Unsafe().Number, uint64(5))
	require.Equal(t, verifier.L2Safe().Number, uint64(0))

	c, e := compressor.NewRatioCompressor(compressor.Config{
		TargetFrameSize:  128_000,
		TargetNumFrames:  1,
		ApproxComprRatio: 1,
	})
	require.NoError(t, e)
	spanBatchBuilder := derive.NewSpanBatchBuilder(sd.RollupCfg.Genesis.L2Time, sd.RollupCfg.L2ChainID)
	// Create new span batch channel
	channelOut, err := derive.NewChannelOut(derive.SpanBatchType, c, spanBatchBuilder)
	require.NoError(t, err)

	for i := uint64(1); i <= sequencer.L2Unsafe().Number; i++ {
		block, err := l2Cl.BlockByNumber(t.Ctx(), new(big.Int).SetUint64(i))
		require.NoError(t, err)
		if i == 2 {
			// Make block B2 as an valid block different with unsafe block
			// Alice makes a L2 tx
			n, err := l2Cl.PendingNonceAt(t.Ctx(), dp.Addresses.Alice)
			require.NoError(t, err)
			validTx := types.MustSignNewTx(dp.Secrets.Alice, signer, &types.DynamicFeeTx{
				ChainID:   sd.L2Cfg.Config.ChainID,
				Nonce:     n,
				GasTipCap: big.NewInt(2 * params.GWei),
				GasFeeCap: new(big.Int).Add(miner.l1Chain.CurrentBlock().BaseFee, big.NewInt(2*params.GWei)),
				Gas:       params.TxGas,
				To:        &dp.Addresses.Bob,
				Value:     e2eutils.Ether(2),
			})
			block = block.WithBody([]*types.Transaction{block.Transactions()[0], validTx}, []*types.Header{})
		}
		if i == 3 {
			// Make block B3 as an invalid block
			invalidTx := testutils.RandomTx(rng, big.NewInt(100), signer)
			block = block.WithBody([]*types.Transaction{block.Transactions()[0], invalidTx}, []*types.Header{})
		}
		// Add A1, B2, B3, B4, B5 into the channel
		_, err = channelOut.AddBlock(block)
		require.NoError(t, err)
	}

	// Submit span batch(A1, B2, invalid B3, B4, B5)
	batcher.l2ChannelOut = channelOut
	batcher.ActL2ChannelClose(t)
	batcher.ActL2BatchSubmit(t)

	miner.ActL1StartBlock(12)(t)
	miner.ActL1IncludeTx(dp.Addresses.Batcher)(t)
	miner.ActL1EndBlock(t)

	// let sequencer process invalid span batch
	sequencer.ActL1HeadSignal(t)
	// before stepping, make sure backupUnsafe is empty
	require.Equal(t, eth.L2BlockRef{}, sequencer.L2BackupUnsafe())
	// pendingSafe must not be advanced as well
	require.Equal(t, sequencer.L2PendingSafe().Number, uint64(0))
	// Preheat engine queue and consume A1 from batch
	for i := 0; i < 4; i++ {
		sequencer.ActL2PipelineStep(t)
	}
	// A1 is valid original block so pendingSafe is advanced
	require.Equal(t, sequencer.L2PendingSafe().Number, uint64(1))
	require.Equal(t, sequencer.L2Unsafe().Number, uint64(5))
	// backupUnsafe is still empty
	require.Equal(t, eth.L2BlockRef{}, sequencer.L2BackupUnsafe())

	// Process B2
	sequencer.ActL2PipelineStep(t)
	sequencer.ActL2PipelineStep(t)
	// B2 is valid different block, triggering unsafe chain reorg
	require.Equal(t, sequencer.L2Unsafe().Number, uint64(2))
	// B2 is valid different block, triggering unsafe block backup
	require.Equal(t, targetUnsafeHeadHash, sequencer.L2BackupUnsafe().Hash)
	// B2 is valid different block, so pendingSafe is advanced
	require.Equal(t, sequencer.L2PendingSafe().Number, uint64(2))
	// try to process invalid leftovers: B3, B4, B5
	sequencer.ActL2PipelineFull(t)
	// backupUnsafe is used because A3 is invalid. Check backupUnsafe is emptied after used
	require.Equal(t, eth.L2BlockRef{}, sequencer.L2BackupUnsafe())

	// check pendingSafe is reset
	require.Equal(t, sequencer.L2PendingSafe().Number, uint64(0))
	// check backupUnsafe is applied
	require.Equal(t, sequencer.L2Unsafe().Hash, targetUnsafeHeadHash)
	require.Equal(t, sequencer.L2Unsafe().Number, uint64(5))
	// safe head cannot be advanced because batch contained invalid blocks
	require.Equal(t, sequencer.L2Safe().Number, uint64(0))

	// let verifier process invalid span batch
	verifier.ActL1HeadSignal(t)
	verifier.ActL2PipelineFull(t)

	// safe head cannot be advanced, while unsafe head not changed
	require.Equal(t, verifier.L2Unsafe().Number, uint64(5))
	require.Equal(t, verifier.L2Safe().Number, uint64(0))
	require.Equal(t, verifier.L2Unsafe().Hash, targetUnsafeHeadHash)

	// Build and submit a span batch with A1 ~ A5
	batcher.ActSubmitAll(t)
	miner.ActL1StartBlock(12)(t)
	miner.ActL1IncludeTx(dp.Addresses.Batcher)(t)
	miner.ActL1EndBlock(t)

	// let sequencer process valid span batch
	sequencer.ActL1HeadSignal(t)
	sequencer.ActL2PipelineFull(t)

	// safe/unsafe head must be advanced
	require.Equal(t, sequencer.L2Unsafe().Number, uint64(5))
	require.Equal(t, sequencer.L2Safe().Number, uint64(5))
	require.Equal(t, sequencer.L2Safe().Hash, targetUnsafeHeadHash)
	// check backupUnsafe is emptied after consolidation
	require.Equal(t, eth.L2BlockRef{}, sequencer.L2BackupUnsafe())

	// let verifier process valid span batch
	verifier.ActL1HeadSignal(t)
	verifier.ActL2PipelineFull(t)

	// safe and unsafe head must be advanced
	require.Equal(t, verifier.L2Unsafe().Number, uint64(5))
	require.Equal(t, verifier.L2Safe().Number, uint64(5))
	require.Equal(t, verifier.L2Safe().Hash, targetUnsafeHeadHash)
	// check backupUnsafe is emptied after consolidation
	require.Equal(t, eth.L2BlockRef{}, verifier.L2BackupUnsafe())
}

func TestBackupUnsafeReorgForkChoiceInputError(gt *testing.T) {
	t := NewDefaultTesting(gt)
	dp := e2eutils.MakeDeployParams(t, defaultRollupTestParams)
	minTs := hexutil.Uint64(0)
	// Activate Delta hardfork
	dp.DeployConfig.L2GenesisDeltaTimeOffset = &minTs
	dp.DeployConfig.L2BlockTime = 2
	sd := e2eutils.Setup(t, dp, defaultAlloc)
	log := testlog.Logger(t, log.LvlInfo)
	_, dp, miner, sequencer, seqEng, verifier, _, batcher := setupReorgTestActors(t, dp, sd, log)
	l2Cl := seqEng.EthClient()
	seqEngCl, err := sources.NewEngineClient(seqEng.RPCClient(), log, nil, sources.EngineClientDefaultConfig(sd.RollupCfg))
	require.NoError(t, err)

	rng := rand.New(rand.NewSource(1234))
	signer := types.LatestSigner(sd.L2Cfg.Config)

	sequencer.ActL2PipelineFull(t)
	verifier.ActL2PipelineFull(t)

	// Create block A1 ~ A5
	for i := 0; i < 5; i++ {
		// Build a L2 block
		sequencer.ActL2StartBlock(t)
		sequencer.ActL2EndBlock(t)

		// Notify new L2 block to verifier by unsafe gossip
		seqHead, err := seqEngCl.PayloadByLabel(t.Ctx(), eth.Unsafe)
		require.NoError(t, err)
		verifier.ActL2UnsafeGossipReceive(seqHead)(t)
	}

	seqHead, err := seqEngCl.PayloadByLabel(t.Ctx(), eth.Unsafe)
	require.NoError(t, err)
	// eventually correct hash for A5
	targetUnsafeHeadHash := seqHead.BlockHash

	// only advance unsafe head to A5
	require.Equal(t, sequencer.L2Unsafe().Number, uint64(5))
	require.Equal(t, sequencer.L2Safe().Number, uint64(0))

	// Handle unsafe payload
	verifier.ActL2PipelineFull(t)
	// only advance unsafe head to A5
	require.Equal(t, verifier.L2Unsafe().Number, uint64(5))
	require.Equal(t, verifier.L2Safe().Number, uint64(0))

	c, e := compressor.NewRatioCompressor(compressor.Config{
		TargetFrameSize:  128_000,
		TargetNumFrames:  1,
		ApproxComprRatio: 1,
	})
	require.NoError(t, e)
	spanBatchBuilder := derive.NewSpanBatchBuilder(sd.RollupCfg.Genesis.L2Time, sd.RollupCfg.L2ChainID)
	// Create new span batch channel
	channelOut, err := derive.NewChannelOut(derive.SpanBatchType, c, spanBatchBuilder)
	require.NoError(t, err)

	for i := uint64(1); i <= sequencer.L2Unsafe().Number; i++ {
		block, err := l2Cl.BlockByNumber(t.Ctx(), new(big.Int).SetUint64(i))
		require.NoError(t, err)
		if i == 2 {
			// Make block B2 as an valid block different with unsafe block
			// Alice makes a L2 tx
			n, err := l2Cl.PendingNonceAt(t.Ctx(), dp.Addresses.Alice)
			require.NoError(t, err)
			validTx := types.MustSignNewTx(dp.Secrets.Alice, signer, &types.DynamicFeeTx{
				ChainID:   sd.L2Cfg.Config.ChainID,
				Nonce:     n,
				GasTipCap: big.NewInt(2 * params.GWei),
				GasFeeCap: new(big.Int).Add(miner.l1Chain.CurrentBlock().BaseFee, big.NewInt(2*params.GWei)),
				Gas:       params.TxGas,
				To:        &dp.Addresses.Bob,
				Value:     e2eutils.Ether(2),
			})
			block = block.WithBody([]*types.Transaction{block.Transactions()[0], validTx}, []*types.Header{})
		}
		if i == 3 {
			// Make block B3 as an invalid block
			invalidTx := testutils.RandomTx(rng, big.NewInt(100), signer)
			block = block.WithBody([]*types.Transaction{block.Transactions()[0], invalidTx}, []*types.Header{})
		}
		// Add A1, B2, B3, B4, B5 into the channel
		_, err = channelOut.AddBlock(block)
		require.NoError(t, err)
	}

	// Submit span batch(A1, B2, invalid B3, B4, B5)
	batcher.l2ChannelOut = channelOut
	batcher.ActL2ChannelClose(t)
	batcher.ActL2BatchSubmit(t)

	miner.ActL1StartBlock(12)(t)
	miner.ActL1IncludeTx(dp.Addresses.Batcher)(t)
	miner.ActL1EndBlock(t)

	// let sequencer process invalid span batch
	sequencer.ActL1HeadSignal(t)
	// before stepping, make sure backupUnsafe is empty
	require.Equal(t, eth.L2BlockRef{}, sequencer.L2BackupUnsafe())
	// pendingSafe must not be advanced as well
	require.Equal(t, sequencer.L2PendingSafe().Number, uint64(0))
	// Preheat engine queue and consume A1 from batch
	for i := 0; i < 4; i++ {
		sequencer.ActL2PipelineStep(t)
	}
	// A1 is valid original block so pendingSafe is advanced
	require.Equal(t, sequencer.L2PendingSafe().Number, uint64(1))
	require.Equal(t, sequencer.L2Unsafe().Number, uint64(5))
	// backupUnsafe is still empty
	require.Equal(t, eth.L2BlockRef{}, sequencer.L2BackupUnsafe())

	// Process B2
	sequencer.ActL2PipelineStep(t)
	sequencer.ActL2PipelineStep(t)
	// B2 is valid different block, triggering unsafe chain reorg
	require.Equal(t, sequencer.L2Unsafe().Number, uint64(2))
	// B2 is valid different block, triggering unsafe block backup
	require.Equal(t, targetUnsafeHeadHash, sequencer.L2BackupUnsafe().Hash)
	// B2 is valid different block, so pendingSafe is advanced
	require.Equal(t, sequencer.L2PendingSafe().Number, uint64(2))

	// B3 is invalid block
	// NextAttributes is called
	sequencer.ActL2PipelineStep(t)
	// forceNextSafeAttributes is called
	sequencer.ActL2PipelineStep(t)
	// mock forkChoiceUpdate error while restoring previous unsafe chain using backupUnsafe.
	seqEng.ActL2RPCFail(t, eth.InputError{Inner: errors.New("mock L2 RPC error"), Code: eth.InvalidForkchoiceState})

	// tryBackupUnsafeReorg is called
	sequencer.ActL2PipelineStep(t)

	// try to process invalid leftovers: B4, B5
	sequencer.ActL2PipelineFull(t)

	// backupUnsafe is not used because forkChoiceUpdate returned an error.
	// Check backupUnsafe is emptied.
	require.Equal(t, eth.L2BlockRef{}, sequencer.L2BackupUnsafe())

	// check pendingSafe is reset
	require.Equal(t, sequencer.L2PendingSafe().Number, uint64(0))
	// unsafe head is not restored due to forkchoiceUpdate error in tryBackupUnsafeReorg
	require.Equal(t, sequencer.L2Unsafe().Number, uint64(2))
	// safe head cannot be advanced because batch contained invalid blocks
	require.Equal(t, sequencer.L2Safe().Number, uint64(0))
}

func TestBackupUnsafeReorgForkChoiceNotInputError(gt *testing.T) {
	t := NewDefaultTesting(gt)
	dp := e2eutils.MakeDeployParams(t, defaultRollupTestParams)
	minTs := hexutil.Uint64(0)
	// Activate Delta hardfork
	dp.DeployConfig.L2GenesisDeltaTimeOffset = &minTs
	dp.DeployConfig.L2BlockTime = 2
	sd := e2eutils.Setup(t, dp, defaultAlloc)
	log := testlog.Logger(t, log.LvlInfo)
	_, dp, miner, sequencer, seqEng, verifier, _, batcher := setupReorgTestActors(t, dp, sd, log)
	l2Cl := seqEng.EthClient()
	seqEngCl, err := sources.NewEngineClient(seqEng.RPCClient(), log, nil, sources.EngineClientDefaultConfig(sd.RollupCfg))
	require.NoError(t, err)

	rng := rand.New(rand.NewSource(1234))
	signer := types.LatestSigner(sd.L2Cfg.Config)

	sequencer.ActL2PipelineFull(t)
	verifier.ActL2PipelineFull(t)

	// Create block A1 ~ A5
	for i := 0; i < 5; i++ {
		// Build a L2 block
		sequencer.ActL2StartBlock(t)
		sequencer.ActL2EndBlock(t)

		// Notify new L2 block to verifier by unsafe gossip
		seqHead, err := seqEngCl.PayloadByLabel(t.Ctx(), eth.Unsafe)
		require.NoError(t, err)
		verifier.ActL2UnsafeGossipReceive(seqHead)(t)
	}

	seqHead, err := seqEngCl.PayloadByLabel(t.Ctx(), eth.Unsafe)
	require.NoError(t, err)
	// eventually correct hash for A5
	targetUnsafeHeadHash := seqHead.BlockHash

	// only advance unsafe head to A5
	require.Equal(t, sequencer.L2Unsafe().Number, uint64(5))
	require.Equal(t, sequencer.L2Safe().Number, uint64(0))

	// Handle unsafe payload
	verifier.ActL2PipelineFull(t)
	// only advance unsafe head to A5
	require.Equal(t, verifier.L2Unsafe().Number, uint64(5))
	require.Equal(t, verifier.L2Safe().Number, uint64(0))

	c, e := compressor.NewRatioCompressor(compressor.Config{
		TargetFrameSize:  128_000,
		TargetNumFrames:  1,
		ApproxComprRatio: 1,
	})
	require.NoError(t, e)
	spanBatchBuilder := derive.NewSpanBatchBuilder(sd.RollupCfg.Genesis.L2Time, sd.RollupCfg.L2ChainID)
	// Create new span batch channel
	channelOut, err := derive.NewChannelOut(derive.SpanBatchType, c, spanBatchBuilder)
	require.NoError(t, err)

	for i := uint64(1); i <= sequencer.L2Unsafe().Number; i++ {
		block, err := l2Cl.BlockByNumber(t.Ctx(), new(big.Int).SetUint64(i))
		require.NoError(t, err)
		if i == 2 {
			// Make block B2 as an valid block different with unsafe block
			// Alice makes a L2 tx
			n, err := l2Cl.PendingNonceAt(t.Ctx(), dp.Addresses.Alice)
			require.NoError(t, err)
			validTx := types.MustSignNewTx(dp.Secrets.Alice, signer, &types.DynamicFeeTx{
				ChainID:   sd.L2Cfg.Config.ChainID,
				Nonce:     n,
				GasTipCap: big.NewInt(2 * params.GWei),
				GasFeeCap: new(big.Int).Add(miner.l1Chain.CurrentBlock().BaseFee, big.NewInt(2*params.GWei)),
				Gas:       params.TxGas,
				To:        &dp.Addresses.Bob,
				Value:     e2eutils.Ether(2),
			})
			block = block.WithBody([]*types.Transaction{block.Transactions()[0], validTx}, []*types.Header{})
		}
		if i == 3 {
			// Make block B3 as an invalid block
			invalidTx := testutils.RandomTx(rng, big.NewInt(100), signer)
			block = block.WithBody([]*types.Transaction{block.Transactions()[0], invalidTx}, []*types.Header{})
		}
		// Add A1, B2, B3, B4, B5 into the channel
		_, err = channelOut.AddBlock(block)
		require.NoError(t, err)
	}

	// Submit span batch(A1, B2, invalid B3, B4, B5)
	batcher.l2ChannelOut = channelOut
	batcher.ActL2ChannelClose(t)
	batcher.ActL2BatchSubmit(t)

	miner.ActL1StartBlock(12)(t)
	miner.ActL1IncludeTx(dp.Addresses.Batcher)(t)
	miner.ActL1EndBlock(t)

	// let sequencer process invalid span batch
	sequencer.ActL1HeadSignal(t)
	// before stepping, make sure backupUnsafe is empty
	require.Equal(t, eth.L2BlockRef{}, sequencer.L2BackupUnsafe())
	// pendingSafe must not be advanced as well
	require.Equal(t, sequencer.L2PendingSafe().Number, uint64(0))
	// Preheat engine queue and consume A1 from batch
	for i := 0; i < 4; i++ {
		sequencer.ActL2PipelineStep(t)
	}
	// A1 is valid original block so pendingSafe is advanced
	require.Equal(t, sequencer.L2PendingSafe().Number, uint64(1))
	require.Equal(t, sequencer.L2Unsafe().Number, uint64(5))
	// backupUnsafe is still empty
	require.Equal(t, eth.L2BlockRef{}, sequencer.L2BackupUnsafe())

	// Process B2
	sequencer.ActL2PipelineStep(t)
	sequencer.ActL2PipelineStep(t)
	// B2 is valid different block, triggering unsafe chain reorg
	require.Equal(t, sequencer.L2Unsafe().Number, uint64(2))
	// B2 is valid different block, triggering unsafe block backup
	require.Equal(t, targetUnsafeHeadHash, sequencer.L2BackupUnsafe().Hash)
	// B2 is valid different block, so pendingSafe is advanced
	require.Equal(t, sequencer.L2PendingSafe().Number, uint64(2))

	// B3 is invalid block
	// NextAttributes is called
	sequencer.ActL2PipelineStep(t)
	// forceNextSafeAttributes is called
	sequencer.ActL2PipelineStep(t)

	serverErrCnt := 2
	for i := 0; i < serverErrCnt; i++ {
		// mock forkChoiceUpdate failure while restoring previous unsafe chain using backupUnsafe.
		seqEng.ActL2RPCFail(t, engine.GenericServerError)
		// tryBackupUnsafeReorg is called - forkChoiceUpdate returns GenericServerError so retry
		sequencer.ActL2PipelineStep(t)
		// backupUnsafeHead not emptied yet
		require.Equal(t, targetUnsafeHeadHash, sequencer.L2BackupUnsafe().Hash)
	}
	// now forkchoice succeeds
	// try to process invalid leftovers: B4, B5
	sequencer.ActL2PipelineFull(t)

	// backupUnsafe is used because forkChoiceUpdate eventually succeeded.
	// Check backupUnsafe is emptied.
	require.Equal(t, eth.L2BlockRef{}, sequencer.L2BackupUnsafe())

	// check pendingSafe is reset
	require.Equal(t, sequencer.L2PendingSafe().Number, uint64(0))
	// check backupUnsafe is applied
	require.Equal(t, sequencer.L2Unsafe().Hash, targetUnsafeHeadHash)
	require.Equal(t, sequencer.L2Unsafe().Number, uint64(5))
	// safe head cannot be advanced because batch contained invalid blocks
	require.Equal(t, sequencer.L2Safe().Number, uint64(0))
}

// TestELSync tests that a verifier will have the EL import the full chain from the sequencer
// when passed a single unsafe block. op-geth can either snap sync or full sync here.
func TestELSync(gt *testing.T) {
	gt.Skip("not implemented yet")
	t := NewDefaultTesting(gt)
	dp := e2eutils.MakeDeployParams(t, defaultRollupTestParams)
	sd := e2eutils.Setup(t, dp, defaultAlloc)
	log := testlog.Logger(t, log.LvlInfo)

	miner, seqEng, sequencer := setupSequencerTest(t, sd, log)
	// Enable engine P2P sync
	_, verifier := setupVerifier(t, sd, log, miner.L1Client(t, sd.RollupCfg), &sync.Config{SyncMode: sync.ELSync})

	seqEngCl, err := sources.NewEngineClient(seqEng.RPCClient(), log, nil, sources.EngineClientDefaultConfig(sd.RollupCfg))
	require.NoError(t, err)

	sequencer.ActL2PipelineFull(t)
	verifier.ActL2PipelineFull(t)

	// Build a L2 block. This block will not be gossiped to verifier, so verifier can not advance chain by itself.
	sequencer.ActL2StartBlock(t)
	sequencer.ActL2EndBlock(t)

	for i := 0; i < 10; i++ {
		// Build a L2 block
		sequencer.ActL2StartBlock(t)
		sequencer.ActL2EndBlock(t)
		// Notify new L2 block to verifier by unsafe gossip
		seqHead, err := seqEngCl.PayloadByLabel(t.Ctx(), eth.Unsafe)
		require.NoError(t, err)
		verifier.ActL2UnsafeGossipReceive(seqHead)(t)
		// Handle unsafe payload
		verifier.ActL2PipelineFull(t)
		// Verifier must advance unsafe head after unsafe gossip.
		require.Equal(t, sequencer.L2Unsafe().Hash, verifier.L2Unsafe().Hash)
	}
	// Actual test flow should be as follows:
	// 1. Build a chain on the sequencer.
	// 2. Gossip only a single final L2 block from the sequencer to the verifier.
	// 3. Assert that the verifier has the full chain.
}

func TestInvalidPayloadInSpanBatch(gt *testing.T) {
	t := NewDefaultTesting(gt)
	dp := e2eutils.MakeDeployParams(t, defaultRollupTestParams)
	minTs := hexutil.Uint64(0)
	// Activate Delta hardfork
	dp.DeployConfig.L2GenesisDeltaTimeOffset = &minTs
	dp.DeployConfig.L2BlockTime = 2
	sd := e2eutils.Setup(t, dp, defaultAlloc)
	log := testlog.Logger(t, log.LvlInfo)
	_, _, miner, sequencer, seqEng, verifier, _, batcher := setupReorgTestActors(t, dp, sd, log)
	l2Cl := seqEng.EthClient()
	rng := rand.New(rand.NewSource(1234))
	signer := types.LatestSigner(sd.L2Cfg.Config)

	sequencer.ActL2PipelineFull(t)
	verifier.ActL2PipelineFull(t)

	c, e := compressor.NewRatioCompressor(compressor.Config{
		TargetFrameSize:  128_000,
		TargetNumFrames:  1,
		ApproxComprRatio: 1,
	})
	require.NoError(t, e)
	spanBatchBuilder := derive.NewSpanBatchBuilder(sd.RollupCfg.Genesis.L2Time, sd.RollupCfg.L2ChainID)
	// Create new span batch channel
	channelOut, err := derive.NewChannelOut(derive.SpanBatchType, c, spanBatchBuilder)
	require.NoError(t, err)

	// Create block A1 ~ A12 for L1 block #0 ~ #2
	miner.ActEmptyBlock(t)
	miner.ActEmptyBlock(t)
	sequencer.ActL1HeadSignal(t)
	sequencer.ActBuildToL1HeadUnsafe(t)

	for i := uint64(1); i <= sequencer.L2Unsafe().Number; i++ {
		block, err := l2Cl.BlockByNumber(t.Ctx(), new(big.Int).SetUint64(i))
		require.NoError(t, err)
		if i == 8 {
			// Make block A8 as an invalid block
			invalidTx := testutils.RandomTx(rng, big.NewInt(100), signer)
			block = block.WithBody([]*types.Transaction{block.Transactions()[0], invalidTx}, []*types.Header{})
		}
		// Add A1 ~ A12 into the channel
		_, err = channelOut.AddBlock(block)
		require.NoError(t, err)
	}

	// Submit span batch(A1, ...,  A7, invalid A8, A9, ..., A12)
	batcher.l2ChannelOut = channelOut
	batcher.ActL2ChannelClose(t)
	batcher.ActL2BatchSubmit(t)

	miner.ActL1StartBlock(12)(t)
	miner.ActL1IncludeTx(dp.Addresses.Batcher)(t)
	miner.ActL1EndBlock(t)
	miner.ActL1SafeNext(t)
	miner.ActL1FinalizeNext(t)

	// After the verifier processed the span batch, only unsafe head should be advanced to A7.
	// Safe head is not updated because the span batch is not fully processed.
	verifier.ActL1HeadSignal(t)
	verifier.ActL2PipelineFull(t)
	require.Equal(t, verifier.L2Unsafe().Number, uint64(7))
	require.Equal(t, verifier.L2Safe().Number, uint64(0))

	// Create new span batch channel
	c, e = compressor.NewRatioCompressor(compressor.Config{
		TargetFrameSize:  128_000,
		TargetNumFrames:  1,
		ApproxComprRatio: 1,
	})
	require.NoError(t, e)
	spanBatchBuilder = derive.NewSpanBatchBuilder(sd.RollupCfg.Genesis.L2Time, sd.RollupCfg.L2ChainID)
	channelOut, err = derive.NewChannelOut(derive.SpanBatchType, c, spanBatchBuilder)
	require.NoError(t, err)

	for i := uint64(1); i <= sequencer.L2Unsafe().Number; i++ {
		block, err := l2Cl.BlockByNumber(t.Ctx(), new(big.Int).SetUint64(i))
		require.NoError(t, err)
		if i == 1 {
			// Create valid TX
			aliceNonce, err := seqEng.EthClient().PendingNonceAt(t.Ctx(), dp.Addresses.Alice)
			require.NoError(t, err)
			data := make([]byte, rand.Intn(100))
			gas, err := core.IntrinsicGas(data, nil, false, true, true, false)
			require.NoError(t, err)
			baseFee := seqEng.l2Chain.CurrentBlock().BaseFee
			tx := types.MustSignNewTx(dp.Secrets.Alice, signer, &types.DynamicFeeTx{
				ChainID:   sd.L2Cfg.Config.ChainID,
				Nonce:     aliceNonce,
				GasTipCap: big.NewInt(2 * params.GWei),
				GasFeeCap: new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), big.NewInt(2*params.GWei)),
				Gas:       gas,
				To:        &dp.Addresses.Bob,
				Value:     big.NewInt(0),
				Data:      data,
			})
			// Create valid new block B1 at the same height as A1
			block = block.WithBody([]*types.Transaction{block.Transactions()[0], tx}, []*types.Header{})
		}
		// Add B1, A2 ~ A12 into the channel
		_, err = channelOut.AddBlock(block)
		require.NoError(t, err)
	}
	// Submit span batch(B1, A2, ... A12)
	batcher.l2ChannelOut = channelOut
	batcher.ActL2ChannelClose(t)
	batcher.ActL2BatchSubmit(t)

	miner.ActL1StartBlock(12)(t)
	miner.ActL1IncludeTx(dp.Addresses.Batcher)(t)
	miner.ActL1EndBlock(t)
	miner.ActL1SafeNext(t)
	miner.ActL1FinalizeNext(t)

	verifier.ActL1HeadSignal(t)
	verifier.ActL2PipelineFull(t)

	// verifier should advance its unsafe and safe head to the height of A12.
	require.Equal(t, verifier.L2Unsafe().Number, uint64(12))
	require.Equal(t, verifier.L2Safe().Number, uint64(12))
}

func TestSpanBatchAtomicity_Consolidation(gt *testing.T) {
	t := NewDefaultTesting(gt)
	dp := e2eutils.MakeDeployParams(t, defaultRollupTestParams)
	minTs := hexutil.Uint64(0)
	// Activate Delta hardfork
	dp.DeployConfig.L2GenesisDeltaTimeOffset = &minTs
	dp.DeployConfig.L2BlockTime = 2
	sd := e2eutils.Setup(t, dp, defaultAlloc)
	log := testlog.Logger(t, log.LvlInfo)
	_, _, miner, sequencer, seqEng, verifier, _, batcher := setupReorgTestActors(t, dp, sd, log)
	seqEngCl, err := sources.NewEngineClient(seqEng.RPCClient(), log, nil, sources.EngineClientDefaultConfig(sd.RollupCfg))
	require.NoError(t, err)

	targetHeadNumber := uint64(6) // L1 block time / L2 block time

	sequencer.ActL2PipelineFull(t)
	verifier.ActL2PipelineFull(t)

	// Create 6 blocks
	miner.ActEmptyBlock(t)
	sequencer.ActL1HeadSignal(t)
	sequencer.ActBuildToL1HeadUnsafe(t)
	require.Equal(t, sequencer.L2Unsafe().Number, targetHeadNumber)

	// Gossip unsafe blocks to the verifier
	for i := uint64(1); i <= sequencer.L2Unsafe().Number; i++ {
		seqHead, err := seqEngCl.PayloadByNumber(t.Ctx(), i)
		require.NoError(t, err)
		verifier.ActL2UnsafeGossipReceive(seqHead)(t)
	}
	verifier.ActL2PipelineFull(t)

	// Check if the verifier's unsafe sync is done
	require.Equal(t, sequencer.L2Unsafe().Hash, verifier.L2Unsafe().Hash)

	// Build and submit a span batch with 6 blocks
	batcher.ActSubmitAll(t)
	miner.ActL1StartBlock(12)(t)
	miner.ActL1IncludeTx(dp.Addresses.Batcher)(t)
	miner.ActL1EndBlock(t)

	// Start verifier safe sync
	verifier.ActL1HeadSignal(t)
	verifier.l2PipelineIdle = false
	for !verifier.l2PipelineIdle {
		verifier.ActL2PipelineStep(t)
		if verifier.L2PendingSafe().Number < targetHeadNumber {
			// If the span batch is not fully processed, the safe head must not advance.
			require.Equal(t, verifier.L2Safe().Number, uint64(0))
		} else {
			// Once the span batch is fully processed, the safe head must advance to the end of span batch.
			require.Equal(t, verifier.L2Safe().Number, targetHeadNumber)
			require.Equal(t, verifier.L2Safe(), verifier.L2PendingSafe())
		}
		// The unsafe head must not be changed
		require.Equal(t, verifier.L2Unsafe(), sequencer.L2Unsafe())
	}
}

func TestSpanBatchAtomicity_ForceAdvance(gt *testing.T) {
	t := NewDefaultTesting(gt)
	dp := e2eutils.MakeDeployParams(t, defaultRollupTestParams)
	minTs := hexutil.Uint64(0)
	// Activate Delta hardfork
	dp.DeployConfig.L2GenesisDeltaTimeOffset = &minTs
	dp.DeployConfig.L2BlockTime = 2
	sd := e2eutils.Setup(t, dp, defaultAlloc)
	log := testlog.Logger(t, log.LvlInfo)
	_, _, miner, sequencer, _, verifier, _, batcher := setupReorgTestActors(t, dp, sd, log)

	targetHeadNumber := uint64(6) // L1 block time / L2 block time

	sequencer.ActL2PipelineFull(t)
	verifier.ActL2PipelineFull(t)
	require.Equal(t, verifier.L2Unsafe().Number, uint64(0))

	// Create 6 blocks
	miner.ActEmptyBlock(t)
	sequencer.ActL1HeadSignal(t)
	sequencer.ActBuildToL1HeadUnsafe(t)
	require.Equal(t, sequencer.L2Unsafe().Number, targetHeadNumber)

	// Build and submit a span batch with 6 blocks
	batcher.ActSubmitAll(t)
	miner.ActL1StartBlock(12)(t)
	miner.ActL1IncludeTx(dp.Addresses.Batcher)(t)
	miner.ActL1EndBlock(t)

	// Start verifier safe sync
	verifier.ActL1HeadSignal(t)
	verifier.l2PipelineIdle = false
	for !verifier.l2PipelineIdle {
		verifier.ActL2PipelineStep(t)
		if verifier.L2PendingSafe().Number < targetHeadNumber {
			// If the span batch is not fully processed, the safe head must not advance.
			require.Equal(t, verifier.L2Safe().Number, uint64(0))
		} else {
			// Once the span batch is fully processed, the safe head must advance to the end of span batch.
			require.Equal(t, verifier.L2Safe().Number, targetHeadNumber)
			require.Equal(t, verifier.L2Safe(), verifier.L2PendingSafe())
		}
		// The unsafe head and the pending safe head must be the same
		require.Equal(t, verifier.L2Unsafe(), verifier.L2PendingSafe())
	}
}

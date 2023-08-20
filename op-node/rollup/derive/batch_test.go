package derive

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/testutils"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
)

func RandomBatchV2(rng *rand.Rand) *BatchData {
	blockCount := uint64(1 + rng.Int()&0xFF)
	originBits := new(big.Int)
	for i := 0; i < int(blockCount); i++ {
		bit := uint(0)
		if testutils.RandomBool(rng) {
			bit = uint(1)
		}
		originBits.SetBit(originBits, i, bit)
	}
	var blockTxCounts []uint64
	totalblockTxCounts := uint64(0)
	for i := 0; i < int(blockCount); i++ {
		blockTxCount := uint64(rng.Intn(16))
		blockTxCounts = append(blockTxCounts, blockTxCount)
		totalblockTxCounts += blockTxCount
	}
	txDatas := make([]hexutil.Bytes, 0)
	txSigs := make([]BatchV2Signature, 0)
	signer := types.NewLondonSigner(big.NewInt(rng.Int63n(1000)))
	for i := 0; i < int(totalblockTxCounts); i++ {
		tx := testutils.RandomTx(rng, new(big.Int).SetUint64(rng.Uint64()), signer)
		batchV2Tx, err := NewBatchV2Tx(*tx)
		if err != nil {
			panic("NewBatchV2Tx:" + err.Error())
		}
		txData, err := batchV2Tx.MarshalBinary()
		if err != nil {
			panic("MarshalBinary:" + err.Error())
		}
		txSig := BatchV2Signature{
			V: rng.Uint64(),
			R: new(uint256.Int).SetBytes32(testutils.RandomData(rng, 32)),
			S: new(uint256.Int).SetBytes32(testutils.RandomData(rng, 32)),
		}
		txDatas = append(txDatas, txData)
		txSigs = append(txSigs, txSig)
	}
	return &BatchData{
		BatchType: BatchV2Type,
		BatchV2: BatchV2{
			BatchV2Prefix: BatchV2Prefix{
				RelTimestamp:  rng.Uint64(),
				L1OriginNum:   rng.Uint64(),
				ParentCheck:   testutils.RandomData(rng, 20),
				L1OriginCheck: testutils.RandomData(rng, 20),
			},
			BatchV2Payload: BatchV2Payload{
				BlockCount:    blockCount,
				OriginBits:    originBits,
				BlockTxCounts: blockTxCounts,
				TxDatas:       txDatas,
				TxSigs:        txSigs,
			},
		},
	}
}

func RandomBatchV1(rng *rand.Rand, txCount int) *BatchData {
	l1Block := types.NewBlock(testutils.RandomHeader(rng),
		nil, nil, nil, trie.NewStackTrie(nil))
	l1InfoTx, err := L1InfoDeposit(0, eth.BlockToInfo(l1Block), eth.SystemConfig{}, testutils.RandomBool(rng))
	if err != nil {
		panic("L1InfoDeposit: " + err.Error())
	}
	l2Block, _ := testutils.RandomBlockPrependTxs(rng, txCount, types.NewTx(l1InfoTx))
	batchV1, _, err := BlockToBatchV1(l2Block)
	if err != nil {
		panic("BlockToBatchV1:" + err.Error())
	}
	return InitBatchDataV1(*batchV1)
}

func TestBatchRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(0xdeadbeef))

	batches := []*BatchData{
		{
			BatchV1: BatchV1{
				ParentHash:   common.Hash{},
				EpochNum:     0,
				Timestamp:    0,
				Transactions: []hexutil.Bytes{},
			},
		},
		{
			BatchV1: BatchV1{
				ParentHash:   common.Hash{31: 0x42},
				EpochNum:     1,
				Timestamp:    1647026951,
				Transactions: []hexutil.Bytes{[]byte{0, 0, 0}, []byte{0x76, 0xfd, 0x7c}},
			},
		},
		RandomBatchV1(rng, 5),
		RandomBatchV1(rng, 7),
		RandomBatchV2(rng),
	}

	for i, batch := range batches {
		enc, err := batch.MarshalBinary()
		assert.NoError(t, err)
		var dec BatchData
		err = dec.UnmarshalBinary(enc)
		assert.NoError(t, err)
		assert.Equal(t, batch, &dec, "Batch not equal test case %v", i)
	}
}

func TestBatchV2Merge(t *testing.T) {
	rng := rand.New(rand.NewSource(0x7331))

	genesisTimeStamp := rng.Uint64()
	l2BlockTime := uint64(2)

	blockCount := 1 + rng.Intn(128)
	var batchV1s []*BatchV1
	for i := 0; i < blockCount; i++ {
		batchV1 := RandomBatchV1(rng, 1+rng.Intn(8)).BatchV1
		batchV1s = append(batchV1s, &batchV1)
	}
	l1BlockNum := rng.Uint64()
	for i := 0; i < blockCount; i++ {
		if rng.Intn(2) == 1 {
			l1BlockNum++
		}
		batchV1s[i].EpochNum = rollup.Epoch(l1BlockNum)
		if i == 0 {
			continue
		}
		batchV1s[i].Timestamp = batchV1s[i-1].Timestamp + l2BlockTime
	}

	var batchV2 BatchV2
	err := batchV2.MergeBatchV1s(batchV1s, uint(0), genesisTimeStamp)
	assert.NoError(t, err)
	assert.Equal(t, batchV2.ParentCheck, batchV1s[0].ParentHash.Bytes()[:20], "invalid parent check")
	assert.Equal(t, batchV2.L1OriginCheck, batchV1s[blockCount-1].EpochHash.Bytes()[:20], "invalid l1 origin check")
	assert.Equal(t, batchV2.RelTimestamp, batchV1s[0].Timestamp-genesisTimeStamp, "invalid relative timestamp")
	for i := 1; i < blockCount; i++ {
		if batchV2.OriginBits.Bit(i) == 1 {
			assert.True(t, batchV1s[i].EpochNum == batchV1s[i-1].EpochNum+1)
		}
	}
	for i := 0; i < len(batchV1s); i++ {
		txCount := len(batchV1s[i].Transactions)
		assert.True(t, txCount == int(batchV2.BlockTxCounts[i]))
	}

	// set invalid tx type to make tx unmarshaling fail
	batchV1s[0].Transactions[0][0] = 0x33
	var batchV2WrongTxType BatchV2
	err = batchV2WrongTxType.MergeBatchV1s(batchV1s, uint(0), genesisTimeStamp)
	require.ErrorContains(t, err, "failed to decode tx")

	var batchV1sEmpty []*BatchV1
	var batchV2Empty BatchV2
	err = batchV2Empty.MergeBatchV1s(batchV1sEmpty, uint(0), genesisTimeStamp)
	require.ErrorContains(t, err, "cannot merge empty batchV1 list")
}

func prepareSplitBatch(rng *rand.Rand, l2BlockTime uint64) ([]eth.L1BlockRef, BatchV2, eth.L2BlockRef, uint64) {
	genesisTimeStamp := rng.Uint64()
	batchV2 := RandomBatchV2(rng).BatchV2
	// recover parentHash
	var parentHash []byte = append(batchV2.ParentCheck, testutils.RandomData(rng, 12)...)
	originBitSum := uint64(0)
	for i := 0; i < int(batchV2.BlockCount); i++ {
		if batchV2.OriginBits.Bit(i) == 1 {
			originBitSum++
		}
	}
	safeHeadOrigin := testutils.RandomBlockRef(rng)
	safeHeadOrigin.Number = batchV2.L1OriginNum - originBitSum
	l1Origins := []eth.L1BlockRef{safeHeadOrigin}
	for i := 0; i < int(originBitSum); i++ {
		l1Origins = append(l1Origins, testutils.NextRandomRef(rng, l1Origins[i]))
	}
	batchV2.L1OriginNum = l1Origins[originBitSum].Number
	batchV2.L1OriginCheck = l1Origins[originBitSum].Hash.Bytes()[:20]

	safeL2head := testutils.RandomL2BlockRef(rng)
	safeL2head.Hash = common.BytesToHash(parentHash)
	safeL2head.L1Origin = safeHeadOrigin.ID()
	// safeL2head must be parent so subtract l2BlockTime
	safeL2head.Time = genesisTimeStamp + batchV2.RelTimestamp - l2BlockTime

	batchV2.DeriveBatchV2Fields(l2BlockTime, genesisTimeStamp)
	return l1Origins, batchV2, safeL2head, genesisTimeStamp
}

func TestBatchV2Split(t *testing.T) {
	rng := rand.New(rand.NewSource(0xbab0bab0))

	l2BlockTime := uint64(2)
	l1Origins, batchV2, _, _ := prepareSplitBatch(rng, l2BlockTime)

	batchV1s, err := batchV2.SplitBatchV2(l1Origins)
	assert.NoError(t, err)

	assert.True(t, len(batchV1s) == int(batchV2.BlockCount))

	for i := 1; i < len(batchV1s); i++ {
		assert.True(t, batchV1s[i].Timestamp == batchV1s[i-1].Timestamp+l2BlockTime)
	}

	l1OriginBlockNumber := batchV1s[0].EpochNum
	for i := 1; i < len(batchV1s); i++ {
		if batchV2.OriginBits.Bit(i) == 1 {
			l1OriginBlockNumber++
		}
		assert.True(t, batchV1s[i].EpochNum == l1OriginBlockNumber)
	}

	for i := 0; i < len(batchV1s); i++ {
		txCount := len(batchV1s[i].Transactions)
		assert.True(t, txCount == int(batchV2.BlockTxCounts[i]))
	}
}

func TestBatchV2SplitValidation(t *testing.T) {
	rng := rand.New(rand.NewSource(0xcafe))

	l2BlockTime := uint64(2)
	l1Origins, batchV2, safeL2head, _ := prepareSplitBatch(rng, l2BlockTime)
	// above datas are sane. Now contaminate with wrong datas

	// set invalid l1 origin check
	batchV2.L1OriginCheck = testutils.RandomData(rng, 20)
	_, err := batchV2.SplitBatchV2CheckValidation(l1Origins, safeL2head)
	require.ErrorContains(t, err, "l1 origin hash mismatch")

	// set invalid parent check
	batchV2.ParentCheck = testutils.RandomData(rng, 20)
	_, err = batchV2.SplitBatchV2CheckValidation(l1Origins, safeL2head)
	require.ErrorContains(t, err, "parent hash mismatch")

	// set invalid tx type to make tx marshaling fail
	batchV2.TxDatas[0][0] = 0x33
	_, err = batchV2.SplitBatchV2CheckValidation(l1Origins, safeL2head)
	require.ErrorContains(t, err, types.ErrTxTypeNotSupported.Error())
}

func TestBatchV2SplitMerge(t *testing.T) {
	rng := rand.New(rand.NewSource(0x13371337))

	l2BlockTime := uint64(2)
	l1Origins, batchV2, safeL2head, genesisTimeStamp := prepareSplitBatch(rng, l2BlockTime)
	originChangedBit := batchV2.OriginBits.Bit(0)
	originBitSum := batchV2.L1OriginNum - safeL2head.L1Origin.Number

	batchV1s, err := batchV2.SplitBatchV2CheckValidation(l1Origins, safeL2head)
	assert.NoError(t, err)

	var batchV2Merged BatchV2
	err = batchV2Merged.MergeBatchV1s(batchV1s, originChangedBit, genesisTimeStamp)
	assert.NoError(t, err)

	assert.Equal(t, batchV2, batchV2Merged, "BatchV2 not equal")

	// check invariants
	// start_epoch_num = safe_l2_head.origin.block_number + (origin_changed_bit ? 1 : 0)
	startEpochNum := uint64(batchV1s[0].EpochNum)
	assert.True(t, startEpochNum == safeL2head.L1Origin.Number+uint64(originChangedBit))
	// end_epoch_num = safe_l2_head.origin.block_number + sum(origin_bits)
	endEpochNum := batchV2.L1OriginNum
	assert.True(t, endEpochNum == safeL2head.L1Origin.Number+originBitSum)
	assert.True(t, endEpochNum == uint64(batchV1s[len(batchV1s)-1].EpochNum))
}

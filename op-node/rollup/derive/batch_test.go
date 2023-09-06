package derive

import (
	"bytes"
	"errors"
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
)

// splitAndValidation splits single RawSpanBatch and initialize SingularBatch lists and validates parentCheck and l1OriginCheck
func splitAndValidation(rawSpanBatch *RawSpanBatch, l1Origins []eth.L1BlockRef, safeL2Head eth.L2BlockRef, blockTime uint64, genesisTimestamp uint64, chainId *big.Int) ([]*SingularBatch, error) {
	spanBatch, err := rawSpanBatch.derive(blockTime, genesisTimestamp, chainId)
	if err != nil {
		return nil, err
	}
	singularBatches, err := spanBatch.GetSingularBatches(l1Origins)
	if err != nil {
		return nil, err
	}
	// set only the first singularBatch's parent hash
	singularBatches[0].ParentHash = safeL2Head.Hash
	if !bytes.Equal(rawSpanBatch.parentCheck, safeL2Head.Hash.Bytes()[:20]) {
		return nil, errors.New("parent hash mismatch")
	}
	l1OriginBlockHash := singularBatches[len(singularBatches)-1].EpochHash
	if !bytes.Equal(rawSpanBatch.l1OriginCheck, l1OriginBlockHash[:20]) {
		return nil, errors.New("l1 origin hash mismatch")
	}
	return singularBatches, nil
}

func RandomRawSpanBatch(rng *rand.Rand, chainId *big.Int) *RawSpanBatch {
	//blockCount := uint64(1 + rng.Int()&0xFF)
	blockCount := uint64(10)
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
	signer := types.NewLondonSigner(chainId)
	var txs [][]byte
	for i := 0; i < int(totalblockTxCounts); i++ {
		tx := testutils.RandomTx(rng, new(big.Int).SetUint64(rng.Uint64()), signer)
		rawTx, err := tx.MarshalBinary()
		if err != nil {
			panic("MarshalBinary:" + err.Error())
		}
		txs = append(txs, rawTx)
	}
	spanBatchTxs, err := newSpanBatchTxs(txs, chainId)
	if err != nil {
		panic(err.Error())
	}
	rawSpanBatch := RawSpanBatch{
		spanBatchPrefix: spanBatchPrefix{
			relTimestamp:  uint64(rng.Uint32()),
			l1OriginNum:   rng.Uint64(),
			parentCheck:   testutils.RandomData(rng, 20),
			l1OriginCheck: testutils.RandomData(rng, 20),
		},
		spanBatchPayload: spanBatchPayload{
			blockCount:    blockCount,
			originBits:    originBits,
			blockTxCounts: blockTxCounts,
			txs:           spanBatchTxs,
		},
	}
	return &rawSpanBatch
}

func RandomSingularBatch(rng *rand.Rand, txCount int) *SingularBatch {
	l1Block := types.NewBlock(testutils.RandomHeader(rng),
		nil, nil, nil, trie.NewStackTrie(nil))
	l1InfoTx, err := L1InfoDeposit(0, eth.BlockToInfo(l1Block), eth.SystemConfig{}, testutils.RandomBool(rng))
	if err != nil {
		panic("L1InfoDeposit: " + err.Error())
	}
	l2Block, _ := testutils.RandomBlockPrependTxs(rng, txCount, types.NewTx(l1InfoTx))
	singularBatch, _, err := BlockToSingularBatch(l2Block)
	if err != nil {
		panic("BlockToSingularBatch:" + err.Error())
	}
	return singularBatch
}

func TestBatchRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(0xdeadbeef))
	blockTime := uint64(2)
	genesisTimestamp := uint64(0)
	chainId := new(big.Int).SetUint64(rng.Uint64())

	batches := []*BatchData{
		{
			SingularBatch: SingularBatch{
				ParentHash:   common.Hash{},
				EpochNum:     0,
				Timestamp:    0,
				Transactions: []hexutil.Bytes{},
			},
		},
		{
			SingularBatch: SingularBatch{
				ParentHash:   common.Hash{31: 0x42},
				EpochNum:     1,
				Timestamp:    1647026951,
				Transactions: []hexutil.Bytes{[]byte{0, 0, 0}, []byte{0x76, 0xfd, 0x7c}},
			},
		},
		NewSingularBatchData(*RandomSingularBatch(rng, 5)),
		NewSingularBatchData(*RandomSingularBatch(rng, 7)),
		NewSpanBatchData(*RandomRawSpanBatch(rng, chainId)),
	}

	for i, batch := range batches {
		enc, err := batch.MarshalBinary()
		assert.NoError(t, err)
		var dec BatchData
		err = dec.UnmarshalBinary(enc)
		assert.NoError(t, err)
		if dec.BatchType == SpanBatchType {
			dec.RawSpanBatch.derive(blockTime, genesisTimestamp, chainId)
		}
		assert.Equal(t, batch, &dec, "Batch not equal test case %v", i)
	}
}

func TestSpanBatchMerge(t *testing.T) {
	rng := rand.New(rand.NewSource(0x7331))

	genesisTimeStamp := rng.Uint64()
	l2BlockTime := uint64(2)
	chainId := new(big.Int).SetUint64(rng.Uint64())

	blockCount := 1 + rng.Intn(128)
	var singularBatchs []*SingularBatch
	for i := 0; i < blockCount; i++ {
		singularBatch := RandomSingularBatch(rng, 1+rng.Intn(8))
		singularBatchs = append(singularBatchs, singularBatch)
	}
	l1BlockNum := rng.Uint64()
	for i := 0; i < blockCount; i++ {
		if rng.Intn(2) == 1 {
			l1BlockNum++
		}
		singularBatchs[i].EpochNum = rollup.Epoch(l1BlockNum)
		if i == 0 {
			continue
		}
		singularBatchs[i].Timestamp = singularBatchs[i-1].Timestamp + l2BlockTime
	}

	spanBatch := NewSpanBatch(singularBatchs)
	rawSpanBatch, err := spanBatch.ToRawSpanBatch(uint(0), genesisTimeStamp, chainId)
	assert.NoError(t, err)
	assert.Equal(t, rawSpanBatch.parentCheck, singularBatchs[0].ParentHash.Bytes()[:20], "invalid parent check")
	assert.Equal(t, rawSpanBatch.l1OriginCheck, singularBatchs[blockCount-1].EpochHash.Bytes()[:20], "invalid l1 origin check")
	assert.Equal(t, rawSpanBatch.relTimestamp, singularBatchs[0].Timestamp-genesisTimeStamp, "invalid relative timestamp")
	for i := 1; i < blockCount; i++ {
		if rawSpanBatch.originBits.Bit(i) == 1 {
			assert.True(t, singularBatchs[i].EpochNum == singularBatchs[i-1].EpochNum+1)
		}
	}
	for i := 0; i < len(singularBatchs); i++ {
		txCount := len(singularBatchs[i].Transactions)
		assert.True(t, txCount == int(rawSpanBatch.blockTxCounts[i]))
	}

	// set invalid tx type to make tx unmarshaling fail
	singularBatchs[0].Transactions[0][0] = 0x33
	spanBatch = NewSpanBatch(singularBatchs)
	_, err = spanBatch.ToRawSpanBatch(uint(0), genesisTimeStamp, chainId)
	require.ErrorContains(t, err, "failed to decode tx")

	//var singularBatchsEmpty []*SingularBatch
	//spanBatch = NewSpanBatch(SpanBatchType, singularBatchsEmpty)
	//_, err = spanBatch.ToRawSpanBatch(uint(0), genesisTimeStamp, chainId)
	//require.ErrorContains(t, err, "cannot merge empty singularBatch list")
}

func prepareSplitBatch(rng *rand.Rand, l2BlockTime uint64, chainId *big.Int) ([]eth.L1BlockRef, *RawSpanBatch, eth.L2BlockRef, uint64) {
	genesisTimeStamp := uint64(rng.Uint32())
	rawSpanBatch := RandomRawSpanBatch(rng, chainId)
	// recover parentHash
	var parentHash []byte = append(rawSpanBatch.parentCheck, testutils.RandomData(rng, 12)...)
	originBitSum := uint64(0)
	for i := 0; i < int(rawSpanBatch.blockCount); i++ {
		if rawSpanBatch.originBits.Bit(i) == 1 {
			originBitSum++
		}
	}
	safeHeadOrigin := testutils.RandomBlockRef(rng)
	safeHeadOrigin.Number = rawSpanBatch.l1OriginNum - originBitSum
	l1Origins := []eth.L1BlockRef{safeHeadOrigin}
	for i := 0; i < int(originBitSum); i++ {
		l1Origins = append(l1Origins, testutils.NextRandomRef(rng, l1Origins[i]))
	}
	rawSpanBatch.l1OriginNum = l1Origins[originBitSum].Number
	rawSpanBatch.l1OriginCheck = l1Origins[originBitSum].Hash.Bytes()[:20]

	safeL2head := testutils.RandomL2BlockRef(rng)
	safeL2head.Hash = common.BytesToHash(parentHash)
	safeL2head.L1Origin = safeHeadOrigin.ID()
	// safeL2head must be parent so subtract l2BlockTime
	safeL2head.Time = genesisTimeStamp + rawSpanBatch.relTimestamp - l2BlockTime

	return l1Origins, rawSpanBatch, safeL2head, genesisTimeStamp
}

func TestSpanBatchSplit(t *testing.T) {
	rng := rand.New(rand.NewSource(0xbab0bab0))

	chainId := new(big.Int).SetUint64(rng.Uint64())
	l2BlockTime := uint64(2)
	l1Origins, rawSpanBatch, _, _ := prepareSplitBatch(rng, l2BlockTime, chainId)

	spanBatch, err := rawSpanBatch.derive(l2BlockTime, genesisTimestamp, chainId)
	assert.NoError(t, err)

	singularBatchs, err := spanBatch.GetSingularBatches(l1Origins)
	assert.NoError(t, err)

	assert.True(t, len(singularBatchs) == int(rawSpanBatch.blockCount))

	for i := 1; i < len(singularBatchs); i++ {
		assert.True(t, singularBatchs[i].Timestamp == singularBatchs[i-1].Timestamp+l2BlockTime)
	}

	l1OriginBlockNumber := singularBatchs[0].EpochNum
	for i := 1; i < len(singularBatchs); i++ {
		if rawSpanBatch.originBits.Bit(i) == 1 {
			l1OriginBlockNumber++
		}
		assert.True(t, singularBatchs[i].EpochNum == l1OriginBlockNumber)
	}

	for i := 0; i < len(singularBatchs); i++ {
		txCount := len(singularBatchs[i].Transactions)
		assert.True(t, txCount == int(rawSpanBatch.blockTxCounts[i]))
	}
}

func TestSpanBatchSplitValidation(t *testing.T) {
	rng := rand.New(rand.NewSource(0xcafe))

	chainId := new(big.Int).SetUint64(rng.Uint64())
	l2BlockTime := uint64(2)
	l1Origins, rawSpanBatch, safeL2head, _ := prepareSplitBatch(rng, l2BlockTime, chainId)
	// above datas are sane. Now contaminate with wrong datas

	// set invalid l1 origin check
	rawSpanBatch.l1OriginCheck = testutils.RandomData(rng, 20)
	_, err := splitAndValidation(rawSpanBatch, l1Origins, safeL2head, l2BlockTime, genesisTimestamp, chainId)
	require.ErrorContains(t, err, "l1 origin hash mismatch")

	// set invalid parent check
	rawSpanBatch.parentCheck = testutils.RandomData(rng, 20)
	_, err = splitAndValidation(rawSpanBatch, l1Origins, safeL2head, l2BlockTime, genesisTimestamp, chainId)
	require.ErrorContains(t, err, "parent hash mismatch")

	// set invalid tx type to make tx marshaling fail
	rawSpanBatch.txs.txDatas[0][0] = 0x33
	_, err = splitAndValidation(rawSpanBatch, l1Origins, safeL2head, l2BlockTime, genesisTimestamp, chainId)
	require.ErrorContains(t, err, types.ErrTxTypeNotSupported.Error())
}

func TestSpanBatchSplitMerge(t *testing.T) {
	rng := rand.New(rand.NewSource(0x13371337))

	chainId := new(big.Int).SetUint64(rng.Uint64())
	l2BlockTime := uint64(2)
	l1Origins, rawSpanBatch, safeL2head, genesisTimeStamp := prepareSplitBatch(rng, l2BlockTime, chainId)
	originChangedBit := rawSpanBatch.originBits.Bit(0)
	originBitSum := rawSpanBatch.l1OriginNum - safeL2head.L1Origin.Number

	singularBatchs, err := splitAndValidation(rawSpanBatch, l1Origins, safeL2head, l2BlockTime, genesisTimeStamp, chainId)
	require.NoError(t, err)

	spanBatch := NewSpanBatch(singularBatchs)
	rawSpanBatchMerged, err := spanBatch.ToRawSpanBatch(originChangedBit, genesisTimeStamp, chainId)
	require.NoError(t, err)

	assert.Equal(t, rawSpanBatch, rawSpanBatchMerged, "RawSpanBatch not equal")

	// check invariants
	//start_epoch_num = safe_l2_head.origin.block_number + (origin_changed_bit ? 1 : 0)
	startEpochNum := uint64(singularBatchs[0].EpochNum)
	assert.True(t, startEpochNum == safeL2head.L1Origin.Number+uint64(originChangedBit))
	// end_epoch_num = safe_l2_head.origin.block_number + sum(origin_bits)
	endEpochNum := rawSpanBatch.l1OriginNum
	assert.True(t, endEpochNum == safeL2head.L1Origin.Number+originBitSum)
	assert.True(t, endEpochNum == uint64(singularBatchs[len(singularBatchs)-1].EpochNum))
}

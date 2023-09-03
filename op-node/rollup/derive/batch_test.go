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

// splitAndValidation splits single SpanBatch and initialize SingularBatch lists and validates ParentCheck and L1OriginCheck
func splitAndValidation(spanBatch *SpanBatch, l1Origins []eth.L1BlockRef, safeL2Head eth.L2BlockRef) ([]*SingularBatch, error) {
	singularBatches, err := spanBatch.SplitSpanBatch(l1Origins)
	if err != nil {
		return nil, err
	}
	// set only the first singularBatch's parent hash
	singularBatches[0].ParentHash = safeL2Head.Hash
	if !spanBatch.CheckParentHash(safeL2Head.Hash) {
		return nil, errors.New("parent hash mismatch")
	}
	l1OriginBlockHash := singularBatches[len(singularBatches)-1].EpochHash
	if !spanBatch.CheckOriginHash(l1OriginBlockHash) {
		return nil, errors.New("l1 origin hash mismatch")
	}
	return singularBatches, nil
}

func RandomSpanBatch(rng *rand.Rand, blockTime uint64, genesisTimestamp uint64, chainId *big.Int) *SpanBatch {
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
	spanBatchTxs, err := NewSpanBatchTxs(txs, chainId)
	if err != nil {
		panic(err.Error())
	}
	spanBatch := SpanBatch{
		batchType: SpanBatchType,
		SpanBatchPrefix: SpanBatchPrefix{
			RelTimestamp:  rng.Uint64(),
			L1OriginNum:   rng.Uint64(),
			ParentCheck:   testutils.RandomData(rng, 20),
			L1OriginCheck: testutils.RandomData(rng, 20),
		},
		SpanBatchPayload: SpanBatchPayload{
			BlockCount:    blockCount,
			OriginBits:    originBits,
			BlockTxCounts: blockTxCounts,
			Txs:           spanBatchTxs,
		},
	}
	spanBatch.DeriveSpanBatchFields(blockTime, genesisTimestamp, chainId)
	return &spanBatch
}

func RandomSpanBatchWithFeeRecipients(rng *rand.Rand, blockTime uint64, genesisTimestamp uint64, chainId *big.Int) *SpanBatch {
	spanBatch := RandomSpanBatch(rng, blockTime, genesisTimestamp, chainId)
	spanBatch.batchType = SpanBatchV2Type
	// FeeRecipent length
	N := int(spanBatch.BlockCount)
	// cardinality of FeeRecipent
	K := 1 + rng.Intn(5)
	var addressSet []common.Address
	for i := 0; i < K; i++ {
		address := testutils.RandomAddress(rng)
		addressSet = append(addressSet, address)
	}
	var addressList []common.Address
	for i := 0; i < N; i++ {
		addressIdx := uint64(rand.Intn(len(addressSet)))
		addressList = append(addressList, addressSet[addressIdx])
	}
	spanBatch.FeeRecipents = addressList
	return spanBatch
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
		NewSpanBatchData(*RandomSpanBatch(rng, blockTime, genesisTimestamp, chainId), SpanBatchType),
		NewSpanBatchData(*RandomSpanBatchWithFeeRecipients(rng, blockTime, genesisTimestamp, chainId), SpanBatchV2Type),
	}

	for i, batch := range batches {
		enc, err := batch.MarshalBinary()
		assert.NoError(t, err)
		var dec BatchData
		err = dec.UnmarshalBinary(enc)
		assert.NoError(t, err)
		if dec.BatchType == SpanBatchType || dec.BatchType == SpanBatchV2Type {
			dec.SpanBatch.DeriveSpanBatchFields(blockTime, genesisTimestamp, chainId)
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

	spanBatch, err := NewSpanBatch(singularBatchs, uint(0), genesisTimeStamp, chainId)
	assert.NoError(t, err)
	assert.Equal(t, spanBatch.ParentCheck, singularBatchs[0].ParentHash.Bytes()[:20], "invalid parent check")
	assert.Equal(t, spanBatch.L1OriginCheck, singularBatchs[blockCount-1].EpochHash.Bytes()[:20], "invalid l1 origin check")
	assert.Equal(t, spanBatch.RelTimestamp, singularBatchs[0].Timestamp-genesisTimeStamp, "invalid relative timestamp")
	for i := 1; i < blockCount; i++ {
		if spanBatch.OriginBits.Bit(i) == 1 {
			assert.True(t, singularBatchs[i].EpochNum == singularBatchs[i-1].EpochNum+1)
		}
	}
	for i := 0; i < len(singularBatchs); i++ {
		txCount := len(singularBatchs[i].Transactions)
		assert.True(t, txCount == int(spanBatch.BlockTxCounts[i]))
	}

	// set invalid tx type to make tx unmarshaling fail
	singularBatchs[0].Transactions[0][0] = 0x33
	_, err = NewSpanBatch(singularBatchs, uint(0), genesisTimeStamp, chainId)
	require.ErrorContains(t, err, "failed to decode tx")

	var singularBatchsEmpty []*SingularBatch
	_, err = NewSpanBatch(singularBatchsEmpty, uint(0), genesisTimeStamp, chainId)
	require.ErrorContains(t, err, "cannot merge empty singularBatch list")
}

func prepareSplitBatch(rng *rand.Rand, l2BlockTime uint64, chainId *big.Int) ([]eth.L1BlockRef, *SpanBatch, eth.L2BlockRef, uint64) {
	genesisTimeStamp := rng.Uint64()
	spanBatch := RandomSpanBatch(rng, l2BlockTime, genesisTimestamp, chainId)
	// recover parentHash
	var parentHash []byte = append(spanBatch.ParentCheck, testutils.RandomData(rng, 12)...)
	originBitSum := uint64(0)
	for i := 0; i < int(spanBatch.BlockCount); i++ {
		if spanBatch.OriginBits.Bit(i) == 1 {
			originBitSum++
		}
	}
	safeHeadOrigin := testutils.RandomBlockRef(rng)
	safeHeadOrigin.Number = spanBatch.L1OriginNum - originBitSum
	l1Origins := []eth.L1BlockRef{safeHeadOrigin}
	for i := 0; i < int(originBitSum); i++ {
		l1Origins = append(l1Origins, testutils.NextRandomRef(rng, l1Origins[i]))
	}
	spanBatch.L1OriginNum = l1Origins[originBitSum].Number
	spanBatch.L1OriginCheck = l1Origins[originBitSum].Hash.Bytes()[:20]

	safeL2head := testutils.RandomL2BlockRef(rng)
	safeL2head.Hash = common.BytesToHash(parentHash)
	safeL2head.L1Origin = safeHeadOrigin.ID()
	// safeL2head must be parent so subtract l2BlockTime
	safeL2head.Time = genesisTimeStamp + spanBatch.RelTimestamp - l2BlockTime

	spanBatch.DeriveSpanBatchFields(l2BlockTime, genesisTimeStamp, chainId)
	return l1Origins, spanBatch, safeL2head, genesisTimeStamp
}

func TestSpanBatchSplit(t *testing.T) {
	rng := rand.New(rand.NewSource(0xbab0bab0))

	chainId := new(big.Int).SetUint64(rng.Uint64())
	l2BlockTime := uint64(2)
	l1Origins, spanBatch, _, _ := prepareSplitBatch(rng, l2BlockTime, chainId)

	singularBatchs, err := spanBatch.SplitSpanBatch(l1Origins)
	assert.NoError(t, err)

	assert.True(t, len(singularBatchs) == int(spanBatch.BlockCount))

	for i := 1; i < len(singularBatchs); i++ {
		assert.True(t, singularBatchs[i].Timestamp == singularBatchs[i-1].Timestamp+l2BlockTime)
	}

	l1OriginBlockNumber := singularBatchs[0].EpochNum
	for i := 1; i < len(singularBatchs); i++ {
		if spanBatch.OriginBits.Bit(i) == 1 {
			l1OriginBlockNumber++
		}
		assert.True(t, singularBatchs[i].EpochNum == l1OriginBlockNumber)
	}

	for i := 0; i < len(singularBatchs); i++ {
		txCount := len(singularBatchs[i].Transactions)
		assert.True(t, txCount == int(spanBatch.BlockTxCounts[i]))
	}
}

func TestSpanBatchSplitValidation(t *testing.T) {
	rng := rand.New(rand.NewSource(0xcafe))

	chainId := new(big.Int).SetUint64(rng.Uint64())
	l2BlockTime := uint64(2)
	l1Origins, spanBatch, safeL2head, _ := prepareSplitBatch(rng, l2BlockTime, chainId)
	// above datas are sane. Now contaminate with wrong datas

	// set invalid l1 origin check
	spanBatch.L1OriginCheck = testutils.RandomData(rng, 20)
	_, err := splitAndValidation(spanBatch, l1Origins, safeL2head)
	require.ErrorContains(t, err, "l1 origin hash mismatch")

	// set invalid parent check
	spanBatch.ParentCheck = testutils.RandomData(rng, 20)
	_, err = splitAndValidation(spanBatch, l1Origins, safeL2head)
	require.ErrorContains(t, err, "parent hash mismatch")

	// set invalid tx type to make tx marshaling fail
	spanBatch.Txs.TxDatas[0][0] = 0x33
	_, err = splitAndValidation(spanBatch, l1Origins, safeL2head)
	require.ErrorContains(t, err, types.ErrTxTypeNotSupported.Error())
}

func TestSpanBatchSplitMerge(t *testing.T) {
	rng := rand.New(rand.NewSource(0x13371337))

	chainId := new(big.Int).SetUint64(rng.Uint64())
	l2BlockTime := uint64(2)
	l1Origins, spanBatch, safeL2head, genesisTimeStamp := prepareSplitBatch(rng, l2BlockTime, chainId)
	originChangedBit := spanBatch.OriginBits.Bit(0)
	originBitSum := spanBatch.L1OriginNum - safeL2head.L1Origin.Number

	singularBatchs, err := splitAndValidation(spanBatch, l1Origins, safeL2head)
	assert.NoError(t, err)

	spanBatchMerged, err := NewSpanBatch(singularBatchs, originChangedBit, genesisTimeStamp, chainId)
	assert.NoError(t, err)

	assert.Equal(t, spanBatch, spanBatchMerged, "SpanBatch not equal")

	// check invariants
	// start_epoch_num = safe_l2_head.origin.block_number + (origin_changed_bit ? 1 : 0)
	startEpochNum := uint64(singularBatchs[0].EpochNum)
	assert.True(t, startEpochNum == safeL2head.L1Origin.Number+uint64(originChangedBit))
	// end_epoch_num = safe_l2_head.origin.block_number + sum(origin_bits)
	endEpochNum := spanBatch.L1OriginNum
	assert.True(t, endEpochNum == safeL2head.L1Origin.Number+originBitSum)
	assert.True(t, endEpochNum == uint64(singularBatchs[len(singularBatchs)-1].EpochNum))
}

func TestSpsanBatchFeeRecipentsRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(0xcafe1338))

	chainId := new(big.Int).SetUint64(rng.Uint64())
	l2BlockTime := uint64(2)
	genesisTimestamp := uint64(0)
	buf := new(bytes.Buffer)
	for i := 0; i < 8; i++ {
		spanBatch := RandomSpanBatchWithFeeRecipients(rng, l2BlockTime, genesisTimestamp, chainId)

		err := spanBatch.EncodeFeeRecipients(buf)
		assert.NoError(t, err)

		feeRecipentsEncoded := buf.Bytes()
		buf.Reset()

		originalFeeRecipents := spanBatch.FeeRecipents[:]
		// remove field
		spanBatch.FeeRecipents = spanBatch.FeeRecipents[:0]
		r := bytes.NewReader(feeRecipentsEncoded)

		err = spanBatch.DecodeFeeRecipients(r)
		assert.NoError(t, err)

		// check repopulated field is consistent
		assert.Equal(t, originalFeeRecipents, spanBatch.FeeRecipents, "fee recipents not equal")
	}
}

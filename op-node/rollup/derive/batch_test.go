package derive

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/testutils"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/stretchr/testify/assert"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

func RandomRawSpanBatch(rng *rand.Rand, chainId *big.Int) *RawSpanBatch {
	blockCount := uint64(1 + rng.Int()&0xFF)
	//blockCount := uint64(10)
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

func RandomSingularBatch(rng *rand.Rand, txCount int, chainID *big.Int) *SingularBatch {
	signer := types.NewLondonSigner(chainID)
	baseFee := big.NewInt(rng.Int63n(300_000_000_000))
	txsEncoded := make([]hexutil.Bytes, 0, txCount)
	// force each tx to have equal chainID
	for i := 0; i < txCount; i++ {
		tx := testutils.RandomTx(rng, baseFee, signer)
		txEncoded, err := tx.MarshalBinary()
		if err != nil {
			panic("tx Marshal binary" + err.Error())
		}
		txsEncoded = append(txsEncoded, hexutil.Bytes(txEncoded))
	}
	return &SingularBatch{
		ParentHash:   testutils.RandomHash(rng),
		EpochNum:     rollup.Epoch(1 + rng.Int63n(100_000_000)),
		EpochHash:    testutils.RandomHash(rng),
		Timestamp:    uint64(rng.Int63n(2_000_000_000)),
		Transactions: txsEncoded,
	}
}

func RandomValidConsecutiveSingularBatches(rng *rand.Rand, chainID *big.Int) []*SingularBatch {
	blockCount := 2 + rng.Intn(128)
	l2BlockTime := uint64(2)

	var singularBatches []*SingularBatch
	for i := 0; i < blockCount; i++ {
		singularBatch := RandomSingularBatch(rng, 1+rng.Intn(8), chainID)
		singularBatches = append(singularBatches, singularBatch)
	}
	l1BlockNum := rng.Uint64()
	// make sure oldest timestamp is large enough
	singularBatches[0].Timestamp += 256
	for i := 0; i < blockCount; i++ {
		originChangedBit := rng.Intn(2)
		if originChangedBit == 1 {
			l1BlockNum++
			singularBatches[i].EpochHash = testutils.RandomHash(rng)
		} else if i > 0 {
			singularBatches[i].EpochHash = singularBatches[i-1].EpochHash
		}
		singularBatches[i].EpochNum = rollup.Epoch(l1BlockNum)
		if i > 0 {
			singularBatches[i].Timestamp = singularBatches[i-1].Timestamp + l2BlockTime
		}
	}
	return singularBatches
}

func TestBatchRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(0xdeadbeef))
	blockTime := uint64(2)
	genesisTimestamp := uint64(0)
	chainID := new(big.Int).SetUint64(rng.Uint64())

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
		NewSingularBatchData(*RandomSingularBatch(rng, 5, chainID)),
		NewSingularBatchData(*RandomSingularBatch(rng, 7, chainID)),
		NewSpanBatchData(*RandomRawSpanBatch(rng, chainID)),
	}

	for i, batch := range batches {
		enc, err := batch.MarshalBinary()
		assert.NoError(t, err)
		var dec BatchData
		err = dec.UnmarshalBinary(enc)
		assert.NoError(t, err)
		if dec.BatchType == SpanBatchType {
			dec.RawSpanBatch.derive(blockTime, genesisTimestamp, chainID)
		}
		assert.Equal(t, batch, &dec, "Batch not equal test case %v", i)
	}
}

func mockL1Origin(rng *rand.Rand, rawSpanBatch *RawSpanBatch, singularBatches []*SingularBatch) []eth.L1BlockRef {
	safeHeadOrigin := testutils.RandomBlockRef(rng)
	safeHeadOrigin.Hash = singularBatches[0].EpochHash
	safeHeadOrigin.Number = uint64(singularBatches[0].EpochNum)

	l1Origins := []eth.L1BlockRef{safeHeadOrigin}
	originBitSum := uint64(0)
	for i := 0; i < int(rawSpanBatch.blockCount); i++ {
		if rawSpanBatch.originBits.Bit(i) == 1 {
			l1Origin := testutils.NextRandomRef(rng, l1Origins[originBitSum])
			originBitSum++
			l1Origin.Hash = singularBatches[i].EpochHash
			l1Origin.Number = uint64(singularBatches[i].EpochNum)
			l1Origins = append(l1Origins, l1Origin)
		}
	}
	return l1Origins
}

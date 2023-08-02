package derive

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/testutils"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"

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
		originBits = originBits.SetBit(originBits, i, bit)
	}
	var blockTxCounts []uint64
	totalblockTxCounts := uint64(0)
	for i := 0; i < int(blockCount); i++ {
		blockTxCount := uint64(rng.Intn(16))
		blockTxCounts = append(blockTxCounts, blockTxCount)
		totalblockTxCounts += blockTxCount
	}
	txDataHeaders := make([]uint64, 0)
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
		txDataHeader := uint64(len(txData))
		txSig := BatchV2Signature{
			V: rng.Uint64(),
			R: new(uint256.Int).SetBytes32(testutils.RandomData(rng, 32)),
			S: new(uint256.Int).SetBytes32(testutils.RandomData(rng, 32)),
		}
		txDataHeaders = append(txDataHeaders, txDataHeader)
		txDatas = append(txDatas, txData)
		txSigs = append(txSigs, txSig)
	}
	return &BatchData{
		BatchType: BatchV2Type,
		BatchV2: BatchV2{
			BatchV2Prefix: BatchV2Prefix{
				Timestamp:     rng.Uint64(),
				L1OriginNum:   rng.Uint64(),
				ParentCheck:   testutils.RandomData(rng, 20),
				L1OriginCheck: testutils.RandomData(rng, 20),
			},
			BatchV2Payload: BatchV2Payload{
				BlockCount:    blockCount,
				OriginBits:    originBits,
				BlockTxCounts: blockTxCounts,
				TxDataHeaders: txDataHeaders,
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
	batchData, _, err := BlockToBatch(l2Block)
	if err != nil {
		panic("BlockToBatch:" + err.Error())
	}
	return batchData
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

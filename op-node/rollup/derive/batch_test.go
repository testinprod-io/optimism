package derive

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum-optimism/optimism/op-node/testutils"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func RandomBatchV2(rng *rand.Rand) BatchV2 {
	blockCount := uint64(rng.Int() & 0xFF)
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
	for i := 0; i < int(totalblockTxCounts); i++ {
		txDataHeader := uint64(rng.Intn(128))
		txData := testutils.RandomData(rng, int(txDataHeader))
		txSig := BatchV2Signature{
			V: rng.Uint64(),
			R: new(uint256.Int).SetBytes32(testutils.RandomData(rng, 32)),
			S: new(uint256.Int).SetBytes32(testutils.RandomData(rng, 32)),
		}
		txDataHeaders = append(txDataHeaders, txDataHeader)
		txDatas = append(txDatas, txData)
		txSigs = append(txSigs, txSig)
	}
	return BatchV2{
		BatchV2Prefix: BatchV2Prefix{
			Timestamp:     rng.Uint64(),
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
	}
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
		{
			BatchType: BatchV2Type,
			BatchV2:   RandomBatchV2(rng),
		},
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

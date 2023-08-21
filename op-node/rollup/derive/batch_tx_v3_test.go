package derive

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum-optimism/optimism/op-node/testutils"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

func TestRecoverV(t *testing.T) {
	rng := rand.New(rand.NewSource(0x12345678))

	chainID := big.NewInt(rng.Int63n(1000))
	signer := types.NewLondonSigner(chainID)
	totalblockTxCount := rng.Intn(100)

	var batchTxsV3 BatchV2TxsV3
	var txTypes []int
	var txSigs []BatchV2Signature
	var originalVs []uint64
	yParityBits := new(big.Int)
	for idx := 0; idx < totalblockTxCount; idx++ {
		tx := testutils.RandomTx(rng, new(big.Int).SetUint64(rng.Uint64()), signer)
		txTypes = append(txTypes, int(tx.Type()))
		var txSig BatchV2Signature
		v, r, s := tx.RawSignatureValues()
		// Do not fill in txSig.V
		txSig.R, _ = uint256.FromBig(r)
		txSig.S, _ = uint256.FromBig(s)
		txSigs = append(txSigs, txSig)
		originalVs = append(originalVs, v.Uint64())
		yParityBit := ConvertVToYParity(v.Uint64(), int(tx.Type()))
		yParityBits.SetBit(yParityBits, idx, yParityBit)
	}

	batchTxsV3.ChainID = chainID
	batchTxsV3.YParityBits = yParityBits
	batchTxsV3.TxSigs = txSigs

	// recover txSig.V
	batchTxsV3.RecoverV(txTypes)

	var recoveredVs []uint64
	for _, txSig := range batchTxsV3.TxSigs {
		recoveredVs = append(recoveredVs, txSig.V)
	}

	assert.Equal(t, originalVs, recoveredVs, "recovered v mismatch")
}

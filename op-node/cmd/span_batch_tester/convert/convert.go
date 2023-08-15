package convert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"path"
	"time"

	"github.com/ethereum-optimism/optimism/op-node/cmd/batch_decoder/reassemble"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Config struct {
	InDirectory      string
	OutDirectory     string
	GenesisTimestamp uint64
	TxType           uint
	ChainID          *big.Int
}

type SpanBatchWithMetadata struct {
	BatchV2     derive.BatchV2
	BatchV2Hash []byte
	L1StartNum  uint64
	L1EndNum    uint64
	L2StartNum  uint64
	L2EndNum    uint64
}

func LoadChannels(dir string) []reassemble.ChannelWithMetadata {
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	var out []reassemble.ChannelWithMetadata
	for _, file := range files {
		f := path.Join(dir, file.Name())
		chm := LoadChannelFile(f)
		out = append(out, chm)
	}
	return out
}

func LoadChannelFile(file string) reassemble.ChannelWithMetadata {
	f, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var chm reassemble.ChannelWithMetadata
	if err := dec.Decode(&chm); err != nil {
		log.Fatalf("Failed to decode %v. Err: %v\n", file, err)
	}
	return chm
}

// GetL2StartParentBlock returns parent L2 block of first L2 block per channel
func GetL2StartParentBlock(client *ethclient.Client, chm reassemble.ChannelWithMetadata) (*types.Block, error) {
	if len(chm.Batches) == 0 {
		return nil, errors.New("channel is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	startParent, err := client.BlockByHash(ctx, chm.Batches[0].ParentHash)
	if err != nil {
		return nil, err
	}
	return startParent, nil
}

// GetOriginChangedBit returns bit indicating L1Origin has changed for given l2 block number compared to its parent
func GetOriginChangedBit(startParent *types.Block, l1OriginHash common.Hash) (uint, error) {
	batch, _, err := derive.BlockToBatch(startParent)
	if err != nil {
		return 0, err
	}
	if batch.BatchV1.EpochHash == l1OriginHash {
		return 0, nil
	}
	return 1, nil
}

// ConvertChannel initialize BatchV2 using BatchV1s per channel
func ConvertChannel(client *ethclient.Client, chm reassemble.ChannelWithMetadata, genesisTimestamp uint64) SpanBatchWithMetadata {
	startParent, err := GetL2StartParentBlock(client, chm)
	if err != nil {
		log.Fatal(err)
	}
	startNum := startParent.NumberU64() + 1
	l1OriginHash := chm.Batches[0].EpochHash
	originChangedBit, err := GetOriginChangedBit(startParent, l1OriginHash)
	if err != nil {
		log.Fatal(err)
	}
	var batchV2 derive.BatchV2
	if err := batchV2.MergeBatchV1s(chm.Batches, originChangedBit, genesisTimestamp); err != nil {
		log.Fatal(err)
	}
	batchV2Encoded, err := batchV2.EncodeBytes()
	if err != nil {
		log.Fatal(err)
	}
	batchV2Hash := crypto.Keccak256(batchV2Encoded)
	return SpanBatchWithMetadata{
		BatchV2:     batchV2,
		BatchV2Hash: batchV2Hash,
		L2StartNum:  startNum,
		L2EndNum:    startNum + uint64(len(chm.Batches)) - 1,
		L1StartNum:  uint64(chm.Batches[0].EpochNum),
		L1EndNum:    uint64(chm.Batches[len(chm.Batches)-1].EpochNum),
	}
}

func writeBatch(bm SpanBatchWithMetadata, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	return enc.Encode(bm)
}

func Convert(client *ethclient.Client, config Config) {
	// update global varibles. Weird but works
	derive.BatchV2TxsType = int(config.TxType)
	derive.ChainID = config.ChainID

	if err := os.MkdirAll(config.OutDirectory, 0750); err != nil {
		log.Fatal(err)
	}
	channels := LoadChannels(config.InDirectory)
	numChannels := len(channels)
	for idx, channel := range channels {
		if !channel.IsReady {
			continue
		}
		bm := ConvertChannel(client, channel, config.GenesisTimestamp)
		filename := path.Join(config.OutDirectory, fmt.Sprintf("%s.json", channel.ID.String()))
		if err := writeBatch(bm, filename); err != nil {
			log.Fatal(err)
		}
		L2BlockCnt := bm.L2EndNum - bm.L2StartNum + 1
		logPrefix := fmt.Sprintf("[%d/%d]", idx+1, numChannels)
		fmt.Printf(logPrefix+" Channel ID: %s, L2StartNum: %d, L2EndNum: %d, L2BlockCnt, %d\n",
			channel.ID.String(), bm.L2StartNum, bm.L2EndNum, L2BlockCnt)
	}
}

package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"path"
	"time"

	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Config struct {
	Start, End   uint64
	ChainID      *big.Int
	OutDirectory string
}

// Batches fetches blocks in the given block range (inclusive to exclusive)
// and creates batches from block information.
func Batches(client *ethclient.Client, config Config) error {
	if err := os.MkdirAll(config.OutDirectory, 0750); err != nil {
		log.Fatal(err)
	}
	number := new(big.Int).SetUint64(config.Start)
	for i := config.Start; i < config.End; i++ {
		if err := fetchBatchesPerBlock(client, number, config); err != nil {
			return err
		}
		number = number.Add(number, common.Big1)
	}
	return nil
}

// fetchBatchesPerBlock gets a block & creates batch.
func fetchBatchesPerBlock(client *ethclient.Client, l2BlockNumber *big.Int, config Config) error {
	// l1Client is not used yet
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	l2Block, err := client.BlockByNumber(ctx, l2BlockNumber)
	if err != nil {
		return err
	}
	fmt.Println("Fetched L2 block: ", l2BlockNumber)
	batch, _, err := derive.BlockToBatch(l2Block)
	if err != nil {
		return err
	}
	filename := path.Join(config.OutDirectory, fmt.Sprintf("%d.json", l2BlockNumber.Uint64()))
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	if err := enc.Encode(batch); err != nil {
		return err
	}
	return nil
}

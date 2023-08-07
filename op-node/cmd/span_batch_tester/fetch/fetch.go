package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"path"
	"sync"
	"time"

	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Config struct {
	Start, End         uint64
	ChainID            *big.Int
	OutDirectory       string
	ConcurrentRequests uint64
}

// Batches fetches blocks in the given block range (inclusive to exclusive)
// and creates batches from block information.
func Batches(client *ethclient.Client, config Config) error {
	if err := os.MkdirAll(config.OutDirectory, 0750); err != nil {
		log.Fatal(err)
	}
	concurrentRequests := config.ConcurrentRequests

	var wg sync.WaitGroup
	for i := config.Start; i < config.End; i += concurrentRequests {
		end := i + concurrentRequests
		if end > config.End {
			end = config.End
		}
		for j := i; j < end; j++ {
			wg.Add(1)
			number := new(big.Int).SetUint64(j)
			go func() {
				defer wg.Done()
				fetchBatchesPerBlock(client, number, config)
			}()
		}
		wg.Wait()
	}
	return nil
}

// fetchBatchesPerBlock gets a block & creates batch.
func fetchBatchesPerBlock(client *ethclient.Client, l2BlockNumber *big.Int, config Config) {
	// l1Client is not used yet
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	l2Block, err := client.BlockByNumber(ctx, l2BlockNumber)
	if err != nil {
		log.Fatal(err)
		return
	}
	fmt.Println("Fetched L2 block: ", l2BlockNumber)
	batch, _, err := derive.BlockToBatch(l2Block)
	if err != nil {
		log.Fatal(err)
		return
	}
	filename := path.Join(config.OutDirectory, fmt.Sprintf("%d.json", l2BlockNumber.Uint64()))
	file, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
		return
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	if err := enc.Encode(batch); err != nil {
		log.Fatal(err)
		return
	}
}

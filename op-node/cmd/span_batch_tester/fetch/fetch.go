package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"log"
	"math/big"
	"os"
	"path"
	"time"

	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/sync/errgroup"
)

type Config struct {
	Start, End         uint64
	ChainID            *big.Int
	OutDirectory       string
	ConcurrentRequests uint64
	EmptyTx            bool
}

// Batches fetches blocks in the given block range (inclusive to exclusive)
// and creates batches from block information.
func Batches(client *ethclient.Client, config Config) error {
	if err := os.MkdirAll(config.OutDirectory, 0750); err != nil {
		log.Fatal(err)
	}
	concurrentRequests := int(config.ConcurrentRequests)

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(concurrentRequests)

	for i := config.Start; i < config.End; i++ {
		if err := ctx.Err(); err != nil {
			break
		}
		number := new(big.Int).SetUint64(i)
		g.Go(func() error {
			if err := fetchBatchesPerBlock(client, number, config); err != nil {
				return fmt.Errorf("error occurred while fetching block %d: %w", i, err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		log.Fatal(err)
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
	if config.EmptyTx {
		batch.BatchV1.Transactions = []hexutil.Bytes{}
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

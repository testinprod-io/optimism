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
	"sync/atomic"
	"time"

	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type TransactionWithMetadata struct {
	TxIndex     uint64             `json:"tx_index"`
	InboxAddr   common.Address     `json:"inbox_address"`
	BlockNumber uint64             `json:"block_number"`
	BlockHash   common.Hash        `json:"block_hash"`
	BlockTime   uint64             `json:"block_time"`
	ChainId     uint64             `json:"chain_id"`
	Sender      common.Address     `json:"sender"`
	ValidSender bool               `json:"valid_sender"`
	Frames      []derive.Frame     `json:"frames"`
	FrameErr    string             `json:"frame_parse_error"`
	ValidFrames bool               `json:"valid_data"`
	Tx          *types.Transaction `json:"tx"`
}

type Config struct {
	Start, End         uint64
	ChainID            *big.Int
	BatchInbox         common.Address
	BatchSenders       map[common.Address]struct{}
	OutDirectory       string
	ConcurrentRequests uint64
}

// Batches fetches & stores all transactions sent to the batch inbox address in
// the given block range (inclusive to exclusive).
// The transactions & metadata are written to the out directory.
func Batches(client *ethclient.Client, config Config) (totalValid, totalInvalid uint64) {
	if err := os.MkdirAll(config.OutDirectory, 0750); err != nil {
		log.Fatal(err)
	}
	signer := types.LatestSignerForChainID(config.ChainID)
	concurrentRequests := config.ConcurrentRequests

	var wg sync.WaitGroup
	for i := config.Start; i < config.End; i += concurrentRequests {
		end := i + concurrentRequests
		if end > config.End {
			end = config.End
		}
		for j := i; j < end; j++ {
			wg.Add(1)
			number := j
			go func() {
				defer wg.Done()
				valid, invalid := fetchBatchesPerBlock(client, number, signer, config)
				atomic.AddUint64(&totalValid, valid)
				atomic.AddUint64(&totalInvalid, invalid)
			}()
		}
		wg.Wait()
	}
	return
}

// fetchBatchesPerBlock gets a block & the parses all of the transactions in the block.
func fetchBatchesPerBlock(client *ethclient.Client, number uint64, signer types.Signer, config Config) (validBatchCount, invalidBatchCount uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	block, err := client.BlockByNumber(ctx, new(big.Int).SetUint64(number))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Fetched block: ", number)
	for i, tx := range block.Transactions() {
		if tx.To() != nil && *tx.To() == config.BatchInbox {
			sender, err := signer.Sender(tx)
			if err != nil {
				log.Fatal(err)
			}
			validSender := true
			if _, ok := config.BatchSenders[sender]; !ok {
				fmt.Printf("Found a transaction (%s) from an invalid sender (%s)\n", tx.Hash().String(), sender.String())
				invalidBatchCount += 1
				validSender = false
			}

			validFrames := true
			frameError := ""
			frames, err := derive.ParseFrames(tx.Data())
			if err != nil {
				fmt.Printf("Found a transaction (%s) with invalid data: %v\n", tx.Hash().String(), err)
				validFrames = false
				frameError = err.Error()
			}

			if validSender && validFrames {
				validBatchCount += 1
			} else {
				invalidBatchCount += 1
			}

			txm := &TransactionWithMetadata{
				Tx:          tx,
				Sender:      sender,
				ValidSender: validSender,
				TxIndex:     uint64(i),
				BlockNumber: block.NumberU64(),
				BlockHash:   block.Hash(),
				BlockTime:   block.Time(),
				ChainId:     config.ChainID.Uint64(),
				InboxAddr:   config.BatchInbox,
				Frames:      frames,
				FrameErr:    frameError,
				ValidFrames: validFrames,
			}
			filename := path.Join(config.OutDirectory, fmt.Sprintf("%s.json", tx.Hash().String()))
			file, err := os.Create(filename)
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()
			enc := json.NewEncoder(file)
			if err := enc.Encode(txm); err != nil {
				log.Fatal(err)
			}
		}
	}
	return
}

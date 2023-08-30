package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/analyze"
	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/convert"
	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/fetch"
	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/format"
	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/merge"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/urfave/cli/v2"
)

func main() {
	app := cli.NewApp()
	app.Name = "span-batch-tester"
	app.Usage = "Optimism Span Batch Tester"
	app.Commands = []*cli.Command{
		{
			Name:  "fetch",
			Usage: "Fetches v0 batches in the specific range",
			Flags: []cli.Flag{
				&cli.IntFlag{
					Name:     "start",
					Required: true,
					Usage:    "First L2 block (inclusive) to fetch",
				},
				&cli.IntFlag{
					Name:     "end",
					Required: true,
					Usage:    "Last L2 block (exclusive) to fetch",
				},
				&cli.StringFlag{
					Name:  "out",
					Value: "/tmp/span_batch_tester/batches_v0_cache",
					Usage: "Cache directory for the found v0 batches",
				},
				&cli.StringFlag{
					Name:     "l2",
					Required: true,
					Usage:    "L2 RPC URL",
					EnvVars:  []string{"L2_RPC"},
				},
				&cli.IntFlag{
					Name:  "concurrent-requests",
					Value: 10,
					Usage: "Concurrency level when fetching L2",
				},
			},
			Action: func(cliCtx *cli.Context) error {
				client, err := ethclient.Dial(cliCtx.String("l2"))
				if err != nil {
					log.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				chainID, err := client.ChainID(ctx)
				if err != nil {
					log.Fatal(err)
				}
				config := fetch.Config{
					Start:              uint64(cliCtx.Int("start")),
					End:                uint64(cliCtx.Int("end")),
					ChainID:            chainID,
					OutDirectory:       cliCtx.String("out"),
					ConcurrentRequests: uint64(cliCtx.Int("concurrent-requests")),
				}
				if err := fetch.Batches(client, config); err != nil {
					return err
				}
				fmt.Printf("Fetched v0 batches in range [%v,%v).\n", config.Start, config.End)
				fmt.Printf("Fetch Config: Chain ID: %v.\n", config.ChainID)
				return nil
			},
		},
		{
			Name:  "convert",
			Usage: "Converts channel with v0 batch to channel with single span batch",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "l2",
					Required: true,
					Usage:    "L2 RPC URL",
					EnvVars:  []string{"L2_RPC"},
				},
				&cli.Uint64Flag{
					Name:     "genesis-timestamp",
					Required: true,
					Usage:    "l2 genesis bedrock timestamp",
					EnvVars:  []string{"GENESIS_TIMESTAMP"},
				},
				&cli.StringFlag{
					Name:  "in",
					Value: "/tmp/span_batch_tester/channel_cache",
					Usage: "Cache directory for the found channels",
				},
				&cli.StringFlag{
					Name:  "out",
					Value: "/tmp/span_batch_tester/span_batch_cache",
					Usage: "Cache directory for the converted batch",
				},
				&cli.UintFlag{
					Name:     "tx-type",
					Required: true,
					Usage:    "Span batch transaction encoding type",
				},
				&cli.IntFlag{
					Name:     "chain-id",
					Required: true,
					Usage:    "L2 chain id",
					Value:    10,
				},
			},
			Action: func(cliCtx *cli.Context) error {
				chainID := big.NewInt(cliCtx.Int64("chain-id"))
				txType := cliCtx.Uint("tx-type")
				if txType > derive.BatchV2TxsV3Type {
					log.Fatal(fmt.Errorf("invalid tx type: %d", txType))
				}
				client, err := ethclient.Dial(cliCtx.String("l2"))
				if err != nil {
					log.Fatal(err)
				}
				config := convert.Config{
					InDirectory:      cliCtx.String("in"),
					OutDirectory:     cliCtx.String("out"),
					GenesisTimestamp: cliCtx.Uint64("genesis-timestamp"),
					ChainID:          chainID,
					TxType:           txType,
				}
				convert.Convert(client, config)
				return nil
			},
		},
		{
			Name:  "analyze",
			Usage: "Analyze span batch",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "in-channel",
					Value: "/tmp/span_batch_tester/channel_cache",
					Usage: "Cache directory for the found channels",
				},
				&cli.StringFlag{
					Name:  "in-span-batch",
					Value: "/tmp/span_batch_tester/span_batch_cache",
					Usage: "Cache directory for the converted batch",
				},
				&cli.StringFlag{
					Name:  "out",
					Value: "/tmp/span_batch_tester/result",
					Usage: "Directory for the analysis result",
				},
				&cli.UintFlag{
					Name:     "tx-type",
					Required: true,
					Usage:    "Span batch transaction encoding type",
				},
				&cli.IntFlag{
					Name:     "chain-id",
					Required: true,
					Usage:    "L2 chain id",
					Value:    10,
				},
			},
			Action: func(cliCtx *cli.Context) error {
				chainID := big.NewInt(cliCtx.Int64("chain-id"))
				txType := cliCtx.Uint("tx-type")
				if txType > derive.BatchV2TxsV3Type {
					log.Fatal(fmt.Errorf("invalid tx type: %d", txType))
				}
				config := analyze.Config{
					InChannelDirectory:   cliCtx.String("in-channel"),
					InSpanBatchDirectory: cliCtx.String("in-span-batch"),
					OutDirectory:         cliCtx.String("out"),
					ChainID:              chainID,
					TxType:               txType,
				}
				analyze.Analyze(config)
				return nil
			},
		},
		{
			Name:  "merge",
			Usage: "Merges v0 batches in the specific range",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "l2",
					Required: true,
					Usage:    "L2 RPC URL",
					EnvVars:  []string{"L2_RPC"},
				},
				&cli.Uint64Flag{
					Name:     "genesis-timestamp",
					Required: true,
					Usage:    "l2 genesis bedrock timestamp",
					EnvVars:  []string{"GENESIS_TIMESTAMP"},
				},
				&cli.IntFlag{
					Name:     "start",
					Required: true,
					Usage:    "First L2 block (inclusive) to start merging",
				},
				&cli.IntFlag{
					Name:     "end",
					Required: true,
					Usage:    "Last L2 block (exclusive) to stop merging",
				},
				&cli.StringFlag{
					Name:  "in",
					Value: "/tmp/span_batch_tester/batches_v0_cache",
					Usage: "Cache directory for the found v0 batches",
				},
				&cli.StringFlag{
					Name:  "out",
					Value: "/tmp/span_batch_tester/merge",
					Usage: "Cache directory for the merged results",
				},
			},
			Action: func(cliCtx *cli.Context) error {
				client, err := ethclient.Dial(cliCtx.String("l2"))
				if err != nil {
					log.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				chainID, err := client.ChainID(ctx)
				if err != nil {
					log.Fatal(err)
				}
				config := merge.Config{
					Start:            uint64(cliCtx.Int("start")),
					End:              uint64(cliCtx.Int("end")),
					InDirectory:      cliCtx.String("in"),
					OutDirectory:     cliCtx.String("out"),
					GenesisTimestamp: cliCtx.Uint64("genesis-timestamp"),
					ChainID:          chainID,
				}
				if err := merge.Merge(client, config); err != nil {
					return err
				}
				fmt.Printf("Merged v0 batches in range [%v,%v).\n", config.Start, config.End)
				return nil
			},
		},
		{
			Name:  "format",
			Usage: "Search for best span batch format",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "in-span-batch",
					Value: "/tmp/span_batch_tester/span_batch_cache",
					Usage: "Cache directory for the converted batch",
				},
				&cli.StringFlag{
					Name:  "out",
					Value: "/tmp/span_batch_tester/format_result",
					Usage: "Directory for the format result",
				},
				&cli.IntFlag{
					Name:     "chain-id",
					Required: true,
					Usage:    "L2 chain id",
					Value:    10,
				},
			},
			Action: func(cliCtx *cli.Context) error {
				// transaction encoding type is always BatchV2TxsV3Type
				derive.BatchV2TxsType = derive.BatchV2TxsV3Type
				chainID := big.NewInt(cliCtx.Int64("chain-id"))
				config := format.Config{
					InSpanBatchDirectory: cliCtx.String("in-span-batch"),
					OutDirectory:         cliCtx.String("out"),
					ChainID:              chainID,
				}
				format.Format(config)
				return nil
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

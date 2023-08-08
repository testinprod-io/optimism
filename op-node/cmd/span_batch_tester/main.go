package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/analyze"
	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/convert"
	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/fetch"
	"github.com/ethereum/go-ethereum/common"
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
			Usage: "Converts channel with v0 batch to channel with single v1 batch",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "l2",
					Required: true,
					Usage:    "L2 RPC URL",
					EnvVars:  []string{"L2_RPC"},
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
			},
			Action: func(cliCtx *cli.Context) error {
				client, err := ethclient.Dial(cliCtx.String("l2"))
				if err != nil {
					log.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				// TODO: fix this(maybe update spec)
				// use first block as genesis because genesis timestamp is zero
				genesisBlock, err := client.BlockByNumber(ctx, common.Big1)
				if err != nil {
					return err
				}
				genesisTimestamp := genesisBlock.Time()

				fmt.Println("genesis: ", genesisTimestamp)

				config := convert.Config{
					InDirectory:      cliCtx.String("in"),
					OutDirectory:     cliCtx.String("out"),
					GenesisTimestamp: genesisTimestamp,
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
					Name:  "in_channel",
					Value: "/tmp/span_batch_tester/channel_cache",
					Usage: "Cache directory for the found channels",
				},
				&cli.StringFlag{
					Name:  "in_span_batch",
					Value: "/tmp/span_batch_tester/span_batch_cache",
					Usage: "Cache directory for the converted batch",
				},
				&cli.StringFlag{
					Name:  "out",
					Value: "/tmp/span_batch_tester/result",
					Usage: "Directory for the analysis result",
				},
			},
			Action: func(cliCtx *cli.Context) error {
				config := analyze.Config{
					InChannelDirectory:            cliCtx.String("in_channel"),
					InSpanBatchDirectoryDirectory: cliCtx.String("in_span_batch"),
					OutDirectory:                  cliCtx.String("out"),
				}
				analyze.Analyze(config)
				return nil
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

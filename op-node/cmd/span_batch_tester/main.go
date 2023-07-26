package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/fetch"
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
					Value: "/tmp/batch_encoder/batches_v0_cache",
					Usage: "Cache directory for the found v0 batches",
				},
				&cli.StringFlag{
					Name:     "l2",
					Required: true,
					Usage:    "L2 RPC URL",
					EnvVars:  []string{"L2_RPC"},
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
					Start:        uint64(cliCtx.Int("start")),
					End:          uint64(cliCtx.Int("end")),
					ChainID:      chainID,
					OutDirectory: cliCtx.String("out"),
				}
				if err := fetch.Batches(client, config); err != nil {
					return err
				}
				fmt.Printf("Fetched v0 batches in range [%v,%v).\n", config.Start, config.End)
				fmt.Printf("Fetch Config: Chain ID: %v.\n", config.ChainID)
				return nil
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

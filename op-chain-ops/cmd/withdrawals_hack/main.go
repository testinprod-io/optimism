package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mattn/go-isatty"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-chain-ops/crossdomain"
	"github.com/ethereum-optimism/optimism/op-chain-ops/genesis/migration"

	"github.com/urfave/cli"
)

func main() {
	log.Root().SetHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(isatty.IsTerminal(os.Stderr.Fd()))))

	app := &cli.App{
		Name:  "migrate-hack",
		Usage: "Migrate a legacy ovm messages",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "evm-messages",
				Usage:    "Path to evm-messages.json",
				Required: true,
			},
		},
		Action: func(ctx *cli.Context) error {
			evmMsgs := ctx.String("evm-messages")
			evmMessages, err := migration.NewSentMessage(evmMsgs)
			if err != nil {
				return err
			}

			migrationData := migration.MigrationData{
				EvmMessages: evmMessages,
			}
			// Convert all input messages into legacy messages. Note that this list is not yet filtered and
			// may be missing some messages or have some extra messages.
			// withdrawal check skipped
			withdrawals, err := migrationData.ToWithdrawals()
			if err != nil {
				return fmt.Errorf("cannot serialize withdrawals: %w", err)
			}
			// https://community.optimism.io/docs/developers/bedrock/public-testnets/#goerli
			// Proxy__OVM_L1CrossDomainMessenger
			l1CrossDomainMessenger := common.HexToAddress("0x5086d1eEF304eb5284A0f6720f79403b4e9bE294")
			crossdomain.MigrateHackWithdrawals(withdrawals, &l1CrossDomainMessenger)

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Crit("error in migration", "err", err)
	}
}

func writeJSON(outfile string, input interface{}) error {
	f, err := os.OpenFile(outfile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(input)
}

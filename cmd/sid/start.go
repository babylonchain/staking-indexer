package main

import (
	"fmt"
	"path/filepath"

	"github.com/babylonchain/vigilante/btcclient"
	"github.com/lightningnetwork/lnd/signal"
	"github.com/urfave/cli"

	"github.com/babylonchain/staking-indexer/btcscanner"
	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/indexer"
	"github.com/babylonchain/staking-indexer/log"
	service "github.com/babylonchain/staking-indexer/server"
	"github.com/babylonchain/staking-indexer/utils"
)

const (
	homeFlag        = "home"
	startHeightFlag = "start-height"
)

var startCommand = cli.Command{
	Name:        "start",
	Usage:       "Start the staking-indexer server",
	Description: "Start the staking-indexer server.",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "The path to the staking indexer home directory",
			Value: config.DefaultHomeDir,
		},
		cli.StringFlag{
			Name:     startHeightFlag,
			Usage:    "The BTC height that the staking indexer starts from",
			Required: true,
		},
	},
	Action: start,
}

func start(ctx *cli.Context) error {
	homePath, err := filepath.Abs(ctx.String(homeFlag))
	if err != nil {
		return err
	}
	homePath = utils.CleanAndExpandPath(homePath)

	startHeight := ctx.Int64(startHeightFlag)
	if startHeight <= 0 {
		return fmt.Errorf("invalid start height %d", startHeight)
	}

	cfg, err := config.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger, err := log.NewRootLoggerWithFile(config.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize the logger: %w", err)
	}

	// create BTC client and connect to BTC server
	btcCfg := config.BTCConfigToVigilanteBTCConfig(cfg.BTCConfig)
	btcClient, err := btcclient.NewWithBlockSubscriber(
		btcCfg,
		cfg.BTCConfig.RetrySleepTime,
		cfg.BTCConfig.MaxRetrySleepTime,
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize the BTC client: %w", err)
	}

	// create BTC scanner
	scanner, err := btcscanner.NewBTCScanner(cfg.BTCScannerConfig, logger, btcClient, uint64(startHeight))
	if err != nil {
		return fmt.Errorf("failed to initialize the BTC scanner: %w", err)
	}

	// create the staking indexer app
	si, err := indexer.NewStakingIndexer(cfg, logger, scanner.ConfirmedBlocksChan())
	if err != nil {
		return fmt.Errorf("failed to initialize the staking indexer app: %w", err)
	}

	// hook interceptor for os signals
	shutdownInterceptor, err := signal.Intercept()
	if err != nil {
		return err
	}

	// create the server
	indexerServer := service.NewStakingIndexerServer(cfg, scanner, si, logger, shutdownInterceptor)

	// run all the services until shutdown
	return indexerServer.RunUntilShutdown()
}

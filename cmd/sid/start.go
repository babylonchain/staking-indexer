package main

import (
	"fmt"
	"path/filepath"

	"github.com/lightningnetwork/lnd/signal"
	"github.com/urfave/cli"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/indexer"
	"github.com/babylonchain/staking-indexer/log"
	service "github.com/babylonchain/staking-indexer/server"
	"github.com/babylonchain/staking-indexer/utils"
)

var startCommand = cli.Command{
	Name:        "start",
	Usage:       "Start the staking-indexer server",
	Description: "Start the staking-indexer server.",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "The path to the staking-indexer home directory",
			Value: config.DefaultHomeDir,
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

	cfg, err := config.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger, err := log.NewRootLoggerWithFile(config.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize the logger: %w", err)
	}

	si, err := indexer.NewStakingIndexer(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create the staking indexer app: %w", err)
	}

	// Hook interceptor for os signals.
	shutdownInterceptor, err := signal.Intercept()
	if err != nil {
		return err
	}

	indexerServer := service.NewStakingIndexerServer(cfg, si, logger, shutdownInterceptor)

	return indexerServer.RunUntilShutdown()
}

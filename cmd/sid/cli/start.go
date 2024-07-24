package cli

import (
	"fmt"
	"path/filepath"

	// "github.com/babylonchain/staking-queue-client/queuemngr"
	"github.com/lightningnetwork/lnd/signal"
	"github.com/scalarorg/staking-queue-client/queuemngr"
	"github.com/urfave/cli"

	"github.com/babylonchain/staking-indexer/btcclient"
	"github.com/babylonchain/staking-indexer/btcscanner"
	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/indexer"
	"github.com/babylonchain/staking-indexer/log"
	"github.com/babylonchain/staking-indexer/params"
	service "github.com/babylonchain/staking-indexer/server"
	"github.com/babylonchain/staking-indexer/utils"
)

const (
	homeFlag        = "home"
	startHeightFlag = "start-height"
	paramsPathFlag  = "params-path"
)

var StartCommand = cli.Command{
	Name:        "start",
	Usage:       "Start the staking-indexer server",
	Description: "Start the staking-indexer server.",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "The path to the staking indexer home directory",
			Value: config.DefaultHomeDir,
		},
		cli.Uint64Flag{
			Name:  startHeightFlag,
			Usage: "The BTC height that the staking indexer starts from",
		},
		cli.StringFlag{
			Name:  paramsPathFlag,
			Usage: "The path to the global params file",
			Value: config.DefaultParamsPath,
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

	// create BTC client and connect to BTC server
	btcClient, err := btcclient.NewBTCClient(
		cfg.BTCConfig,
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize the BTC client: %w", err)
	}

	btcNotifier, err := btcscanner.NewBTCNotifier(
		cfg.BTCConfig,
		&cfg.BTCNetParams,
		&btcscanner.EmptyHintCache{},
	)
	if err != nil {
		return fmt.Errorf("failed to initialize the BTC notifier: %w", err)
	}

	dbBackend, err := cfg.DatabaseConfig.GetDbBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	paramsRetriever, err := params.NewGlobalParamsRetriever(ctx.String(paramsPathFlag))
	if err != nil {
		return fmt.Errorf("failed to initialize params retriever: %w", err)
	}
	versionedParams := paramsRetriever.VersionedParams()

	// create BTC scanner
	// we don't expect the confirmation depth to change across different versions
	// so we can always use the first one
	scanner, err := btcscanner.NewBTCScanner(versionedParams.Versions[0].ConfirmationDepth, logger, btcClient, btcNotifier)
	if err != nil {
		return fmt.Errorf("failed to initialize the BTC scanner: %w", err)
	}

	// create event consumer
	validQueueCfg, err := cfg.QueueConfig.ToQueueClientConfig()
	if err != nil {
		return fmt.Errorf("invalid queue config: %w", err)
	}
	queueConsumer, err := queuemngr.NewQueueManager(validQueueCfg, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize event consumer: %w", err)
	}

	// create the staking indexer app
	si, err := indexer.NewStakingIndexer(cfg, logger, queueConsumer, dbBackend, versionedParams, scanner)
	if err != nil {
		return fmt.Errorf("failed to initialize the staking indexer app: %w", err)
	}

	// get start height
	var startHeight uint64
	if ctx.IsSet(startHeightFlag) {
		startHeight = ctx.Uint64(startHeightFlag)
	} else {
		startHeight = si.GetStartHeight()
	}

	// hook interceptor for os signals
	shutdownInterceptor, err := signal.Intercept()
	if err != nil {
		return err
	}

	// create the server
	indexerServer := service.NewStakingIndexerServer(cfg, queueConsumer, dbBackend, btcNotifier, si, logger, shutdownInterceptor)

	// run all the services until shutdown
	return indexerServer.RunUntilShutdown(startHeight)
}

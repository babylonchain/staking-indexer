package e2etest

import (
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/babylonchain/vigilante/btcclient"
	vtypes "github.com/babylonchain/vigilante/types"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/signal"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/btcscanner"
	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/indexer"
	"github.com/babylonchain/staking-indexer/log"
	"github.com/babylonchain/staking-indexer/server"
)

type TestManager struct {
	Config              *config.Config
	Db                  kvdb.Backend
	Si                  *indexer.StakingIndexer
	serverStopper       *signal.Interceptor
	wg                  *sync.WaitGroup
	BitcoindHandler     *BitcoindTestHandler
	BtcClient           *btcclient.Client
	ConfirmedBlocksChan chan *vtypes.IndexedBlock
}

// bitcoin params used for testing
var (
	r = rand.New(rand.NewSource(time.Now().Unix()))

	regtestParams = &chaincfg.RegressionNetParams

	eventuallyWaitTimeOut = 10 * time.Second
	eventuallyPollTime    = 250 * time.Millisecond
)

func StartManager(t *testing.T) *TestManager {
	h := NewBitcoindHandler(t)
	h.Start()

	dirPath := filepath.Join(os.TempDir(), "stakerd", "e2etest")
	err := os.MkdirAll(dirPath, 0755)
	require.NoError(t, err)

	cfg := defaultStakingIndexerConfig(dirPath)
	logger, err := log.NewRootLoggerWithFile(config.LogFile(dirPath), "debug")
	require.NoError(t, err)

	btcClient, err := btcclient.NewWithBlockSubscriber(
		cfg.BTCConfig.ToVigilanteBTCConfig(),
		cfg.BTCConfig.RetrySleepTime,
		cfg.BTCConfig.MaxRetrySleepTime,
		logger,
	)
	require.NoError(t, err)

	btcNotifier, err := btcclient.NewNodeBackend(
		cfg.BTCConfig.ToBtcNodeBackendConfig(),
		&cfg.BTCNetParams,
		&btcclient.EmptyHintCache{},
	)
	require.NoError(t, err)

	scanner, err := btcscanner.NewBTCScanner(cfg.BTCScannerConfig, logger, btcClient, btcNotifier, 1)
	require.NoError(t, err)

	si, err := indexer.NewStakingIndexer(cfg, logger, scanner.ConfirmedBlocksChan())
	require.NoError(t, err)

	interceptor, err := signal.Intercept()
	require.NoError(t, err)

	service := server.NewStakingIndexerServer(
		cfg,
		scanner,
		si,
		logger,
		interceptor,
	)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := service.RunUntilShutdown()
		if err != nil {
			t.Fatalf("Error running server: %v", err)
		}
	}()
	// Wait for the server to start
	time.Sleep(3 * time.Second)

	return &TestManager{
		Config:              cfg,
		Si:                  si,
		serverStopper:       &interceptor,
		ConfirmedBlocksChan: scanner.ConfirmedBlocksChan(),
		wg:                  &wg,
		BitcoindHandler:     h,
		BtcClient:           btcClient,
	}
}

func (tm *TestManager) Stop() {
	tm.serverStopper.RequestShutdown()
	tm.wg.Wait()
}

func defaultStakingIndexerConfig(homePath string) *config.Config {
	defaultConfig := config.DefaultConfigWithHome(homePath)

	// both wallet and node are bicoind
	defaultConfig.BTCNetParams = *regtestParams

	bitcoindHost := "127.0.0.1:18443"
	bitcoindUser := "user"
	bitcoindPass := "pass"

	defaultConfig.BTCConfig.RPCHost = bitcoindHost
	defaultConfig.BTCConfig.RPCUser = bitcoindUser
	defaultConfig.BTCConfig.RPCPass = bitcoindPass
	defaultConfig.BTCConfig.RPCPolling = true
	defaultConfig.BTCConfig.BlockPollingInterval = 1 * time.Second
	defaultConfig.BTCConfig.TxPollingInterval = 1 * time.Second

	return defaultConfig
}

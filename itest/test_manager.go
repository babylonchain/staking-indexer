package e2etest

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/babylonchain/vigilante/btcclient"
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
	Config          *config.Config
	Db              kvdb.Backend
	Si              *indexer.StakingIndexer
	BS              *btcscanner.BtcScanner
	serverStopper   *signal.Interceptor
	wg              *sync.WaitGroup
	BitcoindHandler *BitcoindTestHandler
	BtcClient       *btcclient.Client
}

// bitcoin params used for testing
var (
	regtestParams = &chaincfg.RegressionNetParams
)

func StartManagerWithNBlocks(t *testing.T, n int) *TestManager {
	h := NewBitcoindHandler(t)
	h.Start()
	passphrase := "pass"
	_ = h.CreateWallet("test-wallet", passphrase)
	h.GenerateBlocks(n)

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
		btcNotifier,
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
		require.NoError(t, err)
	}()
	// Wait for the server to start
	time.Sleep(3 * time.Second)

	return &TestManager{
		Config:          cfg,
		Si:              si,
		BS:              scanner,
		serverStopper:   &interceptor,
		wg:              &wg,
		BitcoindHandler: h,
		BtcClient:       btcClient,
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

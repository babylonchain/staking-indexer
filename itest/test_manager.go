package e2etest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/babylonchain/btc-staker/stakercfg"
	stakertypes "github.com/babylonchain/btc-staker/types"
	"github.com/babylonchain/btc-staker/walletcontroller"
	"github.com/babylonchain/vigilante/btcclient"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/signal"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/btcscanner"
	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/indexer"
	"github.com/babylonchain/staking-indexer/log"
	"github.com/babylonchain/staking-indexer/params"
	"github.com/babylonchain/staking-indexer/queue/client"
	"github.com/babylonchain/staking-indexer/server"
	"github.com/babylonchain/staking-indexer/types"
)

type TestManager struct {
	Config              *config.Config
	Db                  kvdb.Backend
	Si                  *indexer.StakingIndexer
	BS                  *btcscanner.BtcScanner
	WalletPrivKey       *btcec.PrivateKey
	serverStopper       *signal.Interceptor
	wg                  *sync.WaitGroup
	BitcoindHandler     *BitcoindTestHandler
	BtcClient           *btcclient.Client
	StakerWallet        *walletcontroller.RpcWalletController
	MinerAddr           btcutil.Address
	StakingEventQueue   client.QueueClient
	UnbondingEventQueue client.QueueClient
	WithdrawEventQueue  client.QueueClient
}

// bitcoin params used for testing
var (
	regtestParams         = &chaincfg.RegressionNetParams
	eventuallyWaitTimeOut = 1 * time.Minute
	eventuallyPollTime    = 500 * time.Millisecond
	testParamsPath        = "test_params.json"

	// private keys of the covenant committee which correspond to the public keys in test_params.json
	covenantPrivKeysHex = []string{
		"6a2369c2c9f5cd3c4242834228acdc38b73e5b8930f5f4a9b69e6eaf557e60ed",
		"33156cd36d86078c68a4ef9ec22bda49e3b43b420c2b17c4b5d6e23c489a94b6",
		"30e7da183805f9586ca6d3e3fd12450d45143040a14f3770b80aa5dff6243880",
		"94ab69015d3c54867d385b871aebc32283525a16d17720167852e300fdc0761c",
		"1801dae7d983687655ae5c351b836d1f5b3067b581d10058ad505847485bb117",
	}
)

func StartManagerWithNBlocks(t *testing.T, n int) *TestManager {
	h := NewBitcoindHandler(t)
	h.Start()
	passphrase := "pass"
	walletName := "test-wallet"
	_ = h.CreateWallet(walletName, passphrase)
	resp := h.GenerateBlocks(n)

	minerAddressDecoded, err := btcutil.DecodeAddress(resp.Address, regtestParams)
	require.NoError(t, err)

	dirPath := filepath.Join(os.TempDir(), "sid", "e2etest")
	err = os.MkdirAll(dirPath, 0755)
	require.NoError(t, err)

	cfg := defaultStakingIndexerConfig(dirPath)
	logger, err := log.NewRootLoggerWithFile(config.LogFile(dirPath), "debug")
	require.NoError(t, err)

	stakerCfg, _ := defaultStakerConfig(t, passphrase)
	stakerWallet, err := walletcontroller.NewRpcWalletController(stakerCfg)
	require.NoError(t, err)

	err = stakerWallet.UnlockWallet(20)
	require.NoError(t, err)

	walletPrivKey, err := stakerWallet.DumpPrivateKey(minerAddressDecoded)
	require.NoError(t, err)

	// TODO this is not needed after we remove dependency on vigilante
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

	// create event consumer
	queueConsumer, err := setupTestQueueConsumer(t, cfg.QueueConfig)
	require.NoError(t, err)

	paramsRetriever, err := params.NewLocalParamsRetriever(testParamsPath)
	require.NoError(t, err)

	db, err := cfg.DatabaseConfig.GetDbBackend()
	require.NoError(t, err)
	si, err := indexer.NewStakingIndexer(cfg, logger, queueConsumer, db, paramsRetriever.GetParams(), scanner.ConfirmedBlocksChan())
	require.NoError(t, err)

	interceptor, err := signal.Intercept()
	require.NoError(t, err)

	service := server.NewStakingIndexerServer(
		cfg,
		queueConsumer,
		db,
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
		Config:              cfg,
		Si:                  si,
		BS:                  scanner,
		serverStopper:       &interceptor,
		wg:                  &wg,
		BitcoindHandler:     h,
		BtcClient:           btcClient,
		StakerWallet:        stakerWallet,
		WalletPrivKey:       walletPrivKey,
		MinerAddr:           minerAddressDecoded,
		StakingEventQueue:   queueConsumer.StakingQueue,
		UnbondingEventQueue: queueConsumer.UnbondingQueue,
		WithdrawEventQueue:  queueConsumer.WithdrawQueue,
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

func defaultStakerConfig(t *testing.T, passphrase string) (*stakercfg.Config, *rpcclient.Client) {
	defaultConfig := stakercfg.DefaultConfig()

	// both wallet and node are bicoind
	defaultConfig.BtcNodeBackendConfig.ActiveWalletBackend = stakertypes.BitcoindWalletBackend
	defaultConfig.BtcNodeBackendConfig.ActiveNodeBackend = stakertypes.BitcoindNodeBackend
	defaultConfig.ActiveNetParams = *regtestParams

	// Fees configuration
	defaultConfig.BtcNodeBackendConfig.FeeMode = "dynamic"
	defaultConfig.BtcNodeBackendConfig.EstimationMode = stakertypes.DynamicFeeEstimation

	bitcoindHost := "127.0.0.1:18443"
	bitcoindUser := "user"
	bitcoindPass := "pass"

	// Wallet configuration
	defaultConfig.WalletRpcConfig.Host = bitcoindHost
	defaultConfig.WalletRpcConfig.User = bitcoindUser
	defaultConfig.WalletRpcConfig.Pass = bitcoindPass
	defaultConfig.WalletRpcConfig.DisableTls = true
	defaultConfig.WalletConfig.WalletPass = passphrase

	// node configuration
	defaultConfig.BtcNodeBackendConfig.Bitcoind.RPCHost = bitcoindHost
	defaultConfig.BtcNodeBackendConfig.Bitcoind.RPCUser = bitcoindUser
	defaultConfig.BtcNodeBackendConfig.Bitcoind.RPCPass = bitcoindPass

	// Use rpc polling, as it is our default mode and it is a bit more troublesome
	// to configure ZMQ from inside the bitcoind docker container
	defaultConfig.BtcNodeBackendConfig.Bitcoind.RPCPolling = true
	defaultConfig.BtcNodeBackendConfig.Bitcoind.BlockPollingInterval = 1 * time.Second
	defaultConfig.BtcNodeBackendConfig.Bitcoind.TxPollingInterval = 1 * time.Second

	defaultConfig.StakerConfig.BabylonStallingInterval = 1 * time.Second
	defaultConfig.StakerConfig.UnbondingTxCheckInterval = 1 * time.Second

	testRpcClient, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:                 bitcoindHost,
		User:                 bitcoindUser,
		Pass:                 bitcoindPass,
		DisableTLS:           true,
		DisableConnectOnNew:  true,
		DisableAutoReconnect: false,
		// we use post mode as it sure it works with either bitcoind or btcwallet
		// we may need to re-consider it later if we need any notifications
		HTTPPostMode: true,
	}, nil)
	require.NoError(t, err)

	return &defaultConfig, testRpcClient
}

func (tm *TestManager) SendTxWithNConfirmations(t *testing.T, tx *wire.MsgTx, n int) {
	txHash, err := tm.StakerWallet.SendRawTransaction(tx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		txFromMempool := retrieveTransactionFromMempool(t, tm.StakerWallet.Client, []*chainhash.Hash{txHash})
		return len(txFromMempool) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	mBlock := tm.mineNBlock(t, n)
	require.Equal(t, 2, len(mBlock.Transactions))
}

func retrieveTransactionFromMempool(t *testing.T, client *rpcclient.Client, hashes []*chainhash.Hash) []*btcutil.Tx {
	var txes []*btcutil.Tx
	for _, txHash := range hashes {
		tx, err := client.GetRawTransaction(txHash)
		require.NoError(t, err)
		txes = append(txes, tx)
	}
	return txes
}

func (tm *TestManager) mineNBlock(t *testing.T, n int) *wire.MsgBlock {
	resp := tm.BitcoindHandler.GenerateBlocks(n)
	hash, err := chainhash.NewHashFromStr(resp.Blocks[0])
	require.NoError(t, err)
	header, err := tm.StakerWallet.GetBlock(hash)
	require.NoError(t, err)
	return header
}

func (tm *TestManager) WaitForConfirmedTipHeight(t *testing.T, k uint64) {
	currentHeight, err := tm.BitcoindHandler.GetBlockCount()
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		confirmedTip := tm.BS.LastConfirmedHeight()
		return confirmedTip == uint64(currentHeight)-k
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

func (tm *TestManager) CheckNextStakingEvent(t *testing.T, stakingTxHash string) {
	stakingChan, err := tm.StakingEventQueue.ReceiveMessages()
	require.NoError(t, err)
	stakingEventBytes := <-stakingChan
	var activeStakingEvent types.ActiveStakingEvent
	err = json.Unmarshal([]byte(stakingEventBytes.Body), &activeStakingEvent)
	require.NoError(t, err)
	require.Equal(t, stakingTxHash, activeStakingEvent.StakingTxHashHex)
}

func (tm *TestManager) CheckNextUnbondingEvent(t *testing.T, stakingTxHash string) {
	unbondingChan, err := tm.UnbondingEventQueue.ReceiveMessages()
	require.NoError(t, err)
	unbondingEventBytes := <-unbondingChan
	var unbondingEvent types.UnbondingStakingEvent
	err = json.Unmarshal([]byte(unbondingEventBytes.Body), &unbondingEvent)
	require.NoError(t, err)
	require.Equal(t, stakingTxHash, unbondingEvent.StakingTxHashHex)
}

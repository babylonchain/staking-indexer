package e2etest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
	"github.com/babylonchain/staking-indexer/consumer"
	"github.com/babylonchain/staking-indexer/indexer"
	"github.com/babylonchain/staking-indexer/indexerstore"
	"github.com/babylonchain/staking-indexer/log"
	"github.com/babylonchain/staking-indexer/params"
	"github.com/babylonchain/staking-indexer/queue/client"
	"github.com/babylonchain/staking-indexer/server"
	"github.com/babylonchain/staking-indexer/types"
)

type TestManager struct {
	Config             *config.Config
	Db                 kvdb.Backend
	Si                 *indexer.StakingIndexer
	BS                 *btcscanner.BtcScanner
	WalletPrivKey      *btcec.PrivateKey
	serverStopper      *signal.Interceptor
	wg                 *sync.WaitGroup
	BitcoindHandler    *BitcoindTestHandler
	WalletClient       *rpcclient.Client
	MinerAddr          btcutil.Address
	QueueConsumer      *consumer.QueueConsumer
	StakingEventChan   <-chan client.QueueMessage
	UnbondingEventChan <-chan client.QueueMessage
	WithdrawEventChan  <-chan client.QueueMessage
}

// bitcoin params used for testing
var (
	regtestParams         = &chaincfg.RegressionNetParams
	eventuallyWaitTimeOut = 1 * time.Minute
	eventuallyPollTime    = 500 * time.Millisecond
	passphrase            = "pass"
	walletName            = "test-wallet"
)

func StartManagerWithNBlocks(t *testing.T, n int) *TestManager {
	h := NewBitcoindHandler(t)
	h.Start()
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

	rpcclient, err := rpcclient.New(cfg.BTCConfig.ToConnConfig(), nil)
	require.NoError(t, err)
	err = rpcclient.WalletPassphrase(passphrase, 200)
	require.NoError(t, err)
	walletPrivKey, err := rpcclient.DumpPrivKey(minerAddressDecoded)
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

	stakingEventChan, err := queueConsumer.StakingQueue.ReceiveMessages()
	require.NoError(t, err)
	unbondingEventChan, err := queueConsumer.UnbondingQueue.ReceiveMessages()
	require.NoError(t, err)
	withdrawEventChan, err := queueConsumer.WithdrawQueue.ReceiveMessages()
	require.NoError(t, err)

	sysParams, err := params.NewLocalParamsRetriever().GetParams()
	require.NoError(t, err)

	db, err := cfg.DatabaseConfig.GetDbBackend()
	require.NoError(t, err)
	si, err := indexer.NewStakingIndexer(cfg, logger, queueConsumer, db, sysParams, scanner.ConfirmedBlocksChan())
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
		Config:             cfg,
		Si:                 si,
		BS:                 scanner,
		serverStopper:      &interceptor,
		wg:                 &wg,
		BitcoindHandler:    h,
		WalletClient:       rpcclient,
		WalletPrivKey:      walletPrivKey.PrivKey,
		MinerAddr:          minerAddressDecoded,
		QueueConsumer:      queueConsumer,
		StakingEventChan:   stakingEventChan,
		UnbondingEventChan: unbondingEventChan,
		WithdrawEventChan:  withdrawEventChan,
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

func (tm *TestManager) SendTxWithNConfirmations(t *testing.T, tx *wire.MsgTx, n int) {
	txHash, err := tm.WalletClient.SendRawTransaction(tx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		txFromMempool := retrieveTransactionFromMempool(t, tm.WalletClient, []*chainhash.Hash{txHash})
		return len(txFromMempool) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	mBlock := tm.mineNBlock(t, n)
	require.Equal(t, 2, len(mBlock.Transactions))
	t.Logf("sent tx %s with %d confirmations", txHash.String(), n)
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
	header, err := tm.WalletClient.GetBlock(hash)
	require.NoError(t, err)
	return header
}

func (tm *TestManager) WaitForNConfirmations(t *testing.T, n int) {
	currentHeight, err := tm.BitcoindHandler.GetBlockCount()
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		confirmedTip := tm.BS.LastConfirmedHeight()
		return confirmedTip == uint64(currentHeight)-uint64(n)
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

func (tm *TestManager) CheckNextStakingEvent(t *testing.T, stakingTxHash chainhash.Hash) {
	stakingEventBytes := <-tm.StakingEventChan
	var activeStakingEvent types.ActiveStakingEvent
	err := json.Unmarshal([]byte(stakingEventBytes.Body), &activeStakingEvent)
	require.NoError(t, err)
	require.Equal(t, stakingTxHash.String(), activeStakingEvent.StakingTxHashHex)

	err = tm.QueueConsumer.StakingQueue.DeleteMessage(stakingEventBytes.Receipt)
	require.NoError(t, err)
}

func (tm *TestManager) CheckNextUnbondingEvent(t *testing.T, unbondingTxHash chainhash.Hash) {
	unbondingEventBytes := <-tm.UnbondingEventChan
	var unbondingEvent types.UnbondingStakingEvent
	err := json.Unmarshal([]byte(unbondingEventBytes.Body), &unbondingEvent)
	require.NoError(t, err)
	require.Equal(t, unbondingTxHash.String(), unbondingEvent.UnbondingTxHashHex)

	err = tm.QueueConsumer.UnbondingQueue.DeleteMessage(unbondingEventBytes.Receipt)
	require.NoError(t, err)
}

func (tm *TestManager) CheckNextWithdrawEvent(t *testing.T, stakingTxHash chainhash.Hash) {
	withdrawEventBytes := <-tm.WithdrawEventChan
	var withdrawEvent types.WithdrawStakingEvent
	err := json.Unmarshal([]byte(withdrawEventBytes.Body), &withdrawEvent)
	require.NoError(t, err)
	require.Equal(t, stakingTxHash.String(), withdrawEvent.StakingTxHashHex)

	err = tm.QueueConsumer.WithdrawQueue.DeleteMessage(withdrawEventBytes.Receipt)
	require.NoError(t, err)
}

func (tm *TestManager) WaitForStakingTxStored(t *testing.T, txHash chainhash.Hash) {
	var storedTx indexerstore.StoredStakingTransaction
	require.Eventually(t, func() bool {
		storedStakingTx, err := tm.Si.GetStakingTxByHash(&txHash)
		if err != nil {
			return false
		}
		storedTx = *storedStakingTx
		return true
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Equal(t, txHash.String(), storedTx.Tx.TxHash().String())
}

func (tm *TestManager) WaitForUnbondingTxStored(t *testing.T, txHash chainhash.Hash) {
	var storedTx indexerstore.StoredUnbondingTransaction
	require.Eventually(t, func() bool {
		storedUnbondingTx, err := tm.Si.GetUnbondingTxByHash(&txHash)
		if err != nil {
			return false
		}
		storedTx = *storedUnbondingTx
		return true
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Equal(t, txHash.String(), storedTx.Tx.TxHash().String())
}

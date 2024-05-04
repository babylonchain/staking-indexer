package config

import (
	"fmt"
	"time"

	"github.com/btcsuite/btcd/rpcclient"
)

const (
	// default rpc port of signet is 38332
	defaultBitcoindRpcHost        = "127.0.0.1:38332"
	defaultBitcoindRPCUser        = "user"
	defaultBitcoindRPCPass        = "pass"
	defaultBitcoindBlockCacheSize = 20 * 1024 * 1024 // 20 MB
	defaultZMQPubRawBlock         = "tcp://127.0.0.1:29001"
	defaultZMQPubRawTx            = "tcp://127.0.0.1:29002"
	defaultZMQReadDeadline        = 30 * time.Second
	defaultRetrySleepTime         = 5 * time.Second
	defaultMaxRetrySleepTime      = 5 * time.Minute
	// DefaultTxPollingJitter defines the default TxPollingIntervalJitter
	// to be used for bitcoind backend.
	DefaultTxPollingJitter = 0.5
)

// BTCConfig defines configuration for the Bitcoin client
type BTCConfig struct {
	RPCHost              string        `long:"rpchost" description:"The daemon's rpc listening address."`
	RPCUser              string        `long:"rpcuser" description:"Username for RPC connections."`
	RPCPass              string        `long:"rpcpass" default-mask:"-" description:"Password for RPC connections."`
	ZMQPubRawBlock       string        `long:"zmqpubrawblock" description:"The address listening for ZMQ connections to deliver raw block notifications."`
	ZMQPubRawTx          string        `long:"zmqpubrawtx" description:"The address listening for ZMQ connections to deliver raw transaction notifications."`
	ZMQReadDeadline      time.Duration `long:"zmqreaddeadline" description:"The read deadline for reading ZMQ messages from both the block and tx subscriptions."`
	PrunedNodeMaxPeers   int           `long:"pruned-node-max-peers" description:"The maximum number of peers staker will choose from the backend node to retrieve pruned blocks from. This only applies to pruned nodes."`
	RPCPolling           bool          `long:"rpcpolling" description:"Poll the bitcoind RPC interface for block and transaction notifications instead of using the ZMQ interface"`
	BlockPollingInterval time.Duration `long:"blockpollinginterval" description:"The interval that will be used to poll bitcoind for new blocks. Only used if rpcpolling is true."`
	TxPollingInterval    time.Duration `long:"txpollinginterval" description:"The interval that will be used to poll bitcoind for new tx. Only used if rpcpolling is true."`
	BlockCacheSize       uint64        `long:"block-cache-size" description:"Size of the Bitcoin blocks cache."`
	RetrySleepTime       time.Duration `long:"retrysleeptime" description:"Backoff interval for the first retry."`
	MaxRetrySleepTime    time.Duration `long:"maxretrysleeptime" description:"Maximum backoff interval between retries. "`
}

func DefaultBTCConfig() *BTCConfig {
	return &BTCConfig{
		RPCHost:              defaultBitcoindRpcHost,
		RPCUser:              defaultBitcoindRPCUser,
		RPCPass:              defaultBitcoindRPCPass,
		RPCPolling:           true,
		BlockPollingInterval: 30 * time.Second,
		TxPollingInterval:    30 * time.Second,
		BlockCacheSize:       defaultBitcoindBlockCacheSize,
		ZMQPubRawBlock:       defaultZMQPubRawBlock,
		ZMQPubRawTx:          defaultZMQPubRawTx,
		ZMQReadDeadline:      defaultZMQReadDeadline,
		RetrySleepTime:       defaultRetrySleepTime,
		MaxRetrySleepTime:    defaultMaxRetrySleepTime,
	}
}

func (cfg *BTCConfig) ToConnConfig() *rpcclient.ConnConfig {
	return &rpcclient.ConnConfig{
		Host:                 cfg.RPCHost,
		User:                 cfg.RPCUser,
		Pass:                 cfg.RPCPass,
		DisableTLS:           true,
		DisableConnectOnNew:  true,
		DisableAutoReconnect: false,
		// we use post mode as it sure it works with either bitcoind or btcwallet
		// we may need to re-consider it later if we need any notifications
		HTTPPostMode: true,
	}
}

func (cfg *BTCConfig) Validate() error {
	if cfg.RPCHost == "" {
		return fmt.Errorf("RPC host cannot be empty")
	}
	if cfg.RPCUser == "" {
		return fmt.Errorf("RPC user cannot be empty")
	}
	if cfg.RPCPass == "" {
		return fmt.Errorf("RPC password cannot be empty")
	}

	if cfg.BlockPollingInterval <= 0 {
		return fmt.Errorf("block polling interval should be positive")
	}
	if cfg.TxPollingInterval <= 0 {
		return fmt.Errorf("tx polling interval should be positive")
	}

	if cfg.BlockCacheSize <= 0 {
		return fmt.Errorf("block cache size should be positive")
	}

	if cfg.ZMQPubRawBlock == "" {
		return fmt.Errorf("ZMQ publish raw block endpoint cannot not be empty")
	}
	if cfg.ZMQPubRawTx == "" {
		return fmt.Errorf("ZMQ publish raw block endpoint cannot not be empty")
	}
	if cfg.ZMQReadDeadline <= 0 {
		return fmt.Errorf("ZMQ read deadline should be positive")
	}

	if cfg.RetrySleepTime <= 0 {
		return fmt.Errorf("retry sleep time should be positive")
	}
	if cfg.MaxRetrySleepTime <= 0 {
		return fmt.Errorf("max retry sleep time should be positive")
	}

	return nil
}

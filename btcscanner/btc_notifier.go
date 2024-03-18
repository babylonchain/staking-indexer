package btcscanner

import (
	"fmt"
	"net"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/lightningnetwork/lnd/blockcache"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"github.com/lightningnetwork/lnd/chainntnfs/bitcoindnotify"
	"github.com/lightningnetwork/lnd/channeldb"

	"github.com/babylonchain/staking-indexer/config"
)

type BTCNotifier struct {
	chainntnfs.ChainNotifier
}

func BuildDialer(rpcHost string) func(string) (net.Conn, error) {
	return func(addr string) (net.Conn, error) {
		return net.Dial("tcp", rpcHost)
	}
}

func NewBTCNotifier(
	cfg *config.BTCConfig,
	params *chaincfg.Params,
	hintCache *channeldb.HeightHintCache,
) (*BTCNotifier, error) {
	bitcoindCfg := &chain.BitcoindConfig{
		ChainParams:        params,
		Host:               cfg.RPCHost,
		User:               cfg.RPCUser,
		Pass:               cfg.RPCPass,
		Dialer:             BuildDialer(cfg.RPCHost),
		PrunedModeMaxPeers: cfg.PrunedNodeMaxPeers,
	}

	if cfg.RPCPolling {
		bitcoindCfg.PollingConfig = &chain.PollingConfig{
			BlockPollingInterval:    cfg.BlockPollingInterval,
			TxPollingInterval:       cfg.TxPollingInterval,
			TxPollingIntervalJitter: config.DefaultTxPollingJitter,
		}
	} else {
		bitcoindCfg.ZMQConfig = &chain.ZMQConfig{
			ZMQBlockHost:           cfg.ZMQPubRawBlock,
			ZMQTxHost:              cfg.ZMQPubRawTx,
			ZMQReadDeadline:        cfg.ZMQReadDeadline,
			MempoolPollingInterval: cfg.TxPollingInterval,
			PollingIntervalJitter:  config.DefaultTxPollingJitter,
		}
	}

	bitcoindConn, err := chain.NewBitcoindConn(bitcoindCfg)
	if err != nil {
		return nil, err
	}

	if err := bitcoindConn.Start(); err != nil {
		return nil, fmt.Errorf("unable to connect to "+
			"bitcoind: %v", err)
	}

	chainNotifier := bitcoindnotify.New(
		bitcoindConn, params, hintCache,
		hintCache, blockcache.NewBlockCache(cfg.BlockCacheSize),
	)

	return &BTCNotifier{
		ChainNotifier: chainNotifier,
	}, nil

}

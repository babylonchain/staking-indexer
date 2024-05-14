package btcclient

import (
	"fmt"

	"github.com/avast/retry-go/v4"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/types"
	"github.com/babylonchain/staking-indexer/utils"
)

type BTCClient struct {
	client *rpcclient.Client
	logger *zap.Logger
	cfg    *config.BTCConfig
}

func NewBTCClient(cfg *config.BTCConfig, logger *zap.Logger) (*BTCClient, error) {
	c, err := rpcclient.New(cfg.ToConnConfig(), nil)
	if err != nil {
		return nil, err
	}

	return &BTCClient{
		client: c,
		logger: logger,
		cfg:    cfg,
	}, nil
}

func (c *BTCClient) GetTipHeight() (uint64, error) {
	tipHeight, err := c.getBlockCountWithRetry()
	if err != nil {
		return 0, err
	}

	return uint64(tipHeight), nil
}

func (c *BTCClient) GetBlockByHeight(height uint64) (*types.IndexedBlock, error) {
	blockHash, err := c.getBlockHashWithRetry(int64(height))
	if err != nil {
		return nil, fmt.Errorf("failed to get block by height %d: %w", height, err)
	}

	block, err := c.getBlockWithRetry(blockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get block by hash %s: %w", blockHash.String(), err)
	}

	btcTxs := utils.GetWrappedTxs(block)

	return types.NewIndexedBlock(int32(height), &block.Header, btcTxs), nil
}

func (c *BTCClient) getBlockCountWithRetry() (int64, error) {
	var (
		blockCount int64
		err        error
	)

	if err := retry.Do(func() error {
		blockCount, err = c.client.GetBlockCount()
		return err
	}, retry.Attempts(c.cfg.MaxRetryTimes), retry.Delay(c.cfg.RetryInterval), retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			c.logger.Debug(
				"failed to query the block count",
				zap.Uint("attempt", n+1),
				zap.Uint("max_attempts", c.cfg.MaxRetryTimes),
				zap.Error(err),
			)
		})); err != nil {
		return 0, err
	}
	return blockCount, nil
}

func (c *BTCClient) getBlockHashWithRetry(height int64) (*chainhash.Hash, error) {
	var (
		blockHash *chainhash.Hash
		err       error
	)

	if err := retry.Do(func() error {
		blockHash, err = c.client.GetBlockHash(height)
		return err
	}, retry.Attempts(c.cfg.MaxRetryTimes), retry.Delay(c.cfg.RetryInterval), retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			c.logger.Debug(
				"failed to query the block hash",
				zap.Int64("height", height),
				zap.Uint("attempt", n+1),
				zap.Uint("max_attempts", c.cfg.MaxRetryTimes),
				zap.Error(err),
			)
		})); err != nil {
		return nil, err
	}
	return blockHash, nil
}

func (c *BTCClient) getBlockWithRetry(blockHash *chainhash.Hash) (*wire.MsgBlock, error) {
	var (
		block *wire.MsgBlock
		err   error
	)

	if err := retry.Do(func() error {
		block, err = c.client.GetBlock(blockHash)
		return err
	}, retry.Attempts(c.cfg.MaxRetryTimes), retry.Delay(c.cfg.RetryInterval), retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			c.logger.Debug(
				"failed to query the block hash",
				zap.String("hash", blockHash.String()),
				zap.Uint("attempt", n+1),
				zap.Uint("max_attempts", c.cfg.MaxRetryTimes),
				zap.Error(err),
			)
		})); err != nil {
		return nil, err
	}
	return block, nil
}

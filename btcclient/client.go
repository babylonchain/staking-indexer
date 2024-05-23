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

type BlockCountResponse struct {
	count int64
}

func (c *BTCClient) GetTipHeight() (uint64, error) {
	callForBlockCount := func() (*BlockCountResponse, error) {
		count, err := c.client.GetBlockCount()
		if err != nil {
			return nil, err
		}

		return &BlockCountResponse{count: count}, nil
	}

	blockCount, err := clientCallWithRetry(callForBlockCount, c.logger, c.cfg)
	if err != nil {
		return 0, fmt.Errorf("failed to get block count: %w", err)
	}

	return uint64(blockCount.count), nil
}

func (c *BTCClient) GetBlockByHeight(height uint64) (*types.IndexedBlock, error) {
	blockHash, err := c.GetBlockHashByHeight(height)
	if err != nil {
		return nil, err
	}

	callForBlock := func() (*wire.MsgBlock, error) {
		return c.client.GetBlock(blockHash)
	}

	block, err := clientCallWithRetry(callForBlock, c.logger, c.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get block by hash %s: %w", blockHash.String(), err)
	}

	btcTxs := utils.GetWrappedTxs(block)
	return types.NewIndexedBlock(int32(height), &block.Header, btcTxs), nil
}

func (c *BTCClient) GetBlockHashByHeight(height uint64) (*chainhash.Hash, error) {
	callForBlockHash := func() (*chainhash.Hash, error) {
		return c.client.GetBlockHash(int64(height))
	}

	blockHash, err := clientCallWithRetry(callForBlockHash, c.logger, c.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get block by height %d: %w", height, err)
	}

	return blockHash, nil
}

func (c *BTCClient) GetBlockHeaderByHeight(height uint64) (*wire.BlockHeader, error) {
	blockHash, err := c.GetBlockHashByHeight(height)
	if err != nil {
		return nil, err
	}

	callForBlockHeader := func() (*wire.BlockHeader, error) {
		return c.client.GetBlockHeader(blockHash)
	}

	header, err := clientCallWithRetry(callForBlockHeader, c.logger, c.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get block header by hash %s: %w", blockHash.String(), err)
	}

	return header, nil
}

func clientCallWithRetry[T any](
	call retry.RetryableFuncWithData[*T], logger *zap.Logger, cfg *config.BTCConfig,
) (*T, error) {
	result, err := retry.DoWithData(call, retry.Attempts(cfg.MaxRetryTimes), retry.Delay(cfg.RetryInterval), retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			logger.Debug(
				"failed to call the RPC client",
				zap.Uint("attempt", n+1),
				zap.Uint("max_attempts", cfg.MaxRetryTimes),
				zap.Error(err),
			)
		}))

	if err != nil {
		return nil, err
	}
	return result, nil
}

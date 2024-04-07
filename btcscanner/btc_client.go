package btcscanner

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/types"
	"github.com/babylonchain/staking-indexer/utils"
)

type Client interface {
	GetTipHeight() (uint64, error)
	GetBlockByHeight(height uint64) (*types.IndexedBlock, error)
}

type BTCClient struct {
	client *rpcclient.Client
}

func NewBTCClient(cfg *config.BTCConfig) (*BTCClient, error) {
	c, err := rpcclient.New(cfg.ToConnConfig(), nil)
	if err != nil {
		return nil, err
	}

	return &BTCClient{client: c}, nil
}

func (c *BTCClient) GetTipHeight() (uint64, error) {
	tipHeight, err := c.client.GetBlockCount()
	if err != nil {
		return 0, err
	}

	return uint64(tipHeight), nil
}

func (c *BTCClient) GetBlockByHeight(height uint64) (*types.IndexedBlock, error) {
	blockHash, err := c.client.GetBlockHash(int64(height))
	if err != nil {
		return nil, fmt.Errorf("failed to get block by height %d: %w", height, err)
	}

	return c.GetBlockByHash(blockHash)
}

func (c *BTCClient) GetBlockByHash(blockHash *chainhash.Hash) (*types.IndexedBlock, error) {
	blockInfo, err := c.client.GetBlockVerbose(blockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get block verbose by hash %s: %w", blockHash.String(), err)
	}

	block, err := c.client.GetBlock(blockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get block by hash %s: %w", blockHash.String(), err)
	}

	btcTxs := utils.GetWrappedTxs(block)
	return types.NewIndexedBlock(int32(blockInfo.Height), &block.Header, btcTxs), nil
}

package btcscanner

import (
	"fmt"
	"sync"
	"time"

	notifier "github.com/lightningnetwork/lnd/chainntnfs"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/types"
)

type BtcScanner interface {
	Start(startHeight uint64) error
	ConfirmedBlocksChan() chan *types.IndexedBlock
	LastConfirmedHeight() uint64
	GetUnconfirmedBlocks() ([]*types.IndexedBlock, error)
	IsSynced() bool
	Stop() error
}

type BtcPoller struct {
	logger *zap.Logger

	// connect to BTC node
	btcClient   Client
	btcNotifier notifier.ChainNotifier

	paramsVersions *types.ParamsVersions

	// the current tip BTC block
	confirmedTipBlock *types.IndexedBlock

	// cache of a sequence of unconfirmed blocks
	unconfirmedBlockCache *BTCCache

	// communicate with the consumer
	confirmedBlocksChan chan *types.IndexedBlock

	wg        sync.WaitGroup
	isStarted *atomic.Bool
	isSynced  *atomic.Bool
	quit      chan struct{}
}

func NewBTCScanner(
	paramsVersions *types.ParamsVersions,
	logger *zap.Logger,
	btcClient Client,
	btcNotifier notifier.ChainNotifier,
) (*BtcPoller, error) {
	unconfirmedBlockCache, err := NewBTCCache(defaultMaxEntries)
	if err != nil {
		return nil, fmt.Errorf("failed to create BTC cache for tail blocks: %w", err)
	}

	return &BtcPoller{
		logger:                logger.With(zap.String("module", "btcscanner")),
		btcClient:             btcClient,
		btcNotifier:           btcNotifier,
		paramsVersions:        paramsVersions,
		confirmedBlocksChan:   make(chan *types.IndexedBlock),
		unconfirmedBlockCache: unconfirmedBlockCache,
		isSynced:              atomic.NewBool(false),
		isStarted:             atomic.NewBool(false),
		quit:                  make(chan struct{}),
	}, nil
}

// Start starts the scanning process from the last confirmed height + 1
func (bs *BtcPoller) Start(startHeight uint64) error {
	if bs.isStarted.Swap(true) {
		return fmt.Errorf("the BTC scanner is already started")
	}

	bs.logger.Info("starting the BTC scanner")

	if err := bs.Bootstrap(startHeight); err != nil {
		return fmt.Errorf("failed to bootstrap with height %d", startHeight)
	}

	if err := bs.waitUntilActivation(); err != nil {
		return err
	}

	bs.logger.Info("starting the BTC scanner", zap.Uint64("start_height", startHeight))

	blockEventNotifier, err := bs.btcNotifier.RegisterBlockEpochNtfn(nil)
	if err != nil {
		return fmt.Errorf("failed to register BTC notifier")
	}

	bs.logger.Info("BTC notifier registered")

	// start handling new blocks
	bs.wg.Add(1)
	go bs.blockEventLoop(blockEventNotifier)

	bs.logger.Info("the BTC scanner is started")

	return nil
}

func (bs *BtcPoller) waitUntilActivation() error {
	tipHeight, err := bs.btcClient.GetTipHeight()
	if err != nil {
		return fmt.Errorf("failed to get the current BTC tip height")
	}
	activationHeight := bs.paramsVersions.ParamsVersions[0].ActivationHeight

	for {
		if tipHeight >= activationHeight {
			break
		}

		bs.logger.Info("waiting to reach the earliest activation height",
			zap.Uint64("tip_height", tipHeight),
			zap.Uint64("activation_height", activationHeight))
		time.Sleep(1 * time.Second)
	}

	return nil
}

// Bootstrap syncs with BTC by getting the confirmed blocks and the caching the unconfirmed blocks
func (bs *BtcPoller) Bootstrap(startHeight uint64) error {
	var (
		tipConfirmedHeight uint64
		confirmedBlock     *types.IndexedBlock
		err                error
	)

	if bs.isSynced.Load() {
		// the scanner is already synced
		return nil
	}
	defer bs.isSynced.Store(true)

	bs.logger.Info("the bootstrapping starts", zap.Uint64("start height", startHeight))

	// clear all the blocks in the cache to avoid forks
	bs.unconfirmedBlockCache.RemoveAll()

	tipHeight, err := bs.btcClient.GetTipHeight()
	if err != nil {
		return fmt.Errorf("cannot get the best BTC block")
	}

	params, err := bs.paramsVersions.GetParamsForBTCHeight(int32(tipHeight))
	if err != nil {
		return fmt.Errorf("cannot get the global parameters for height %d", tipHeight)
	}

	if tipHeight < uint64(params.ConfirmationDepth) {
		tipConfirmedHeight = 0
	} else {
		tipConfirmedHeight = tipHeight - uint64(params.ConfirmationDepth) + 1
	}

	// process confirmed blocks
	for i := startHeight; i <= tipConfirmedHeight; i++ {
		// TODO should retry here
		ib, err := bs.btcClient.GetBlockByHeight(i)
		if err != nil {
			return fmt.Errorf("cannot get the block at height %d: %w", i, err)
		}

		// this is a confirmed block
		confirmedBlock = ib

		// if the scanner was bootstrapped before, the new confirmed canonical chain must connect to the previous one
		if bs.confirmedTipBlock != nil {
			confirmedTipHash := bs.confirmedTipBlock.BlockHash()
			if !confirmedTipHash.IsEqual(&confirmedBlock.Header.PrevBlock) {
				return fmt.Errorf("invalid canonical chain")
			}
		}

		bs.sendConfirmedBlocksToChan([]*types.IndexedBlock{confirmedBlock})
	}

	if bs.confirmedTipBlock == nil && tipConfirmedHeight != 0 {
		// TODO should retry here
		ib, err := bs.btcClient.GetBlockByHeight(tipConfirmedHeight)
		if err != nil {
			return fmt.Errorf("cannot get the block at height %d: %w", tipConfirmedHeight, err)
		}
		bs.confirmedTipBlock = ib
	}

	// add unconfirmed blocks into the cache
	for i := tipConfirmedHeight + 1; i <= tipHeight; i++ {
		// TODO should retry here
		ib, err := bs.btcClient.GetBlockByHeight(i)
		if err != nil {
			return fmt.Errorf("cannot get the block at height %d: %w", i, err)
		}

		// the unconfirmed blocks should follow the canonical chain
		tipCache := bs.unconfirmedBlockCache.Tip()
		if tipCache != nil {
			tipHash := tipCache.BlockHash()
			if !tipHash.IsEqual(&ib.Header.PrevBlock) {
				return fmt.Errorf("the block is not connected to the cache tip")
			}
		}

		if err := bs.unconfirmedBlockCache.Add(ib); err != nil {
			return fmt.Errorf("failed to add the block %d to cache: %w", ib.Height, err)
		}
	}

	bs.logger.Info("bootstrapping is finished",
		zap.Uint64("tip_confirmed_height", tipConfirmedHeight),
		zap.Uint64("tip_unconfirmed_height", tipHeight))

	return nil
}

func (bs *BtcPoller) sendConfirmedBlocksToChan(blocks []*types.IndexedBlock) {
	bs.confirmedTipBlock = blocks[len(blocks)-1]
	for i := 0; i < len(blocks); i++ {
		bs.confirmedBlocksChan <- blocks[i]
	}
}

func (bs *BtcPoller) GetUnconfirmedBlocks() ([]*types.IndexedBlock, error) {
	tipBlock := bs.unconfirmedBlockCache.Tip()
	params, err := bs.paramsVersions.GetParamsForBTCHeight(tipBlock.Height)
	if err != nil {
		return nil, fmt.Errorf("failed to get params for height %d: %w", tipBlock.Height, err)
	}

	lastBlocks := bs.unconfirmedBlockCache.GetLastBlocks(int(params.ConfirmationDepth) - 1)

	return lastBlocks, nil
}

func (bs *BtcPoller) ConfirmedBlocksChan() chan *types.IndexedBlock {
	return bs.confirmedBlocksChan
}

func (bs *BtcPoller) LastConfirmedHeight() uint64 {
	if bs.confirmedTipBlock == nil {
		return 0
	}
	return uint64(bs.confirmedTipBlock.Height)
}

func (bs *BtcPoller) IsSynced() bool {
	return bs.isSynced.Load()
}

func (bs *BtcPoller) Stop() error {
	if !bs.isStarted.Swap(false) {
		return nil
	}

	close(bs.quit)
	bs.wg.Wait()

	bs.logger.Info("the BTC scanner is successfully stopped")

	return nil
}

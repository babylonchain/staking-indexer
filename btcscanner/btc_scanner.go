package btcscanner

import (
	"fmt"
	"sync"

	"github.com/babylonchain/vigilante/btcclient"
	vtypes "github.com/babylonchain/vigilante/types"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
)

type BtcScanner struct {
	logger *zap.Logger

	// connect to BTC node
	btcClient btcclient.BTCClient

	// the BTC height the scanner starts
	baseHeight uint64
	// the BTC confirmation depth
	k uint64

	confirmedTipBlock *vtypes.IndexedBlock

	// cache of a sequence of unconfirmed blocks
	unconfirmedBlockCache *vtypes.BTCCache

	// communicate with the consumer
	confirmedBlocksChan chan *vtypes.IndexedBlock

	wg        sync.WaitGroup
	isStarted *atomic.Bool
	isSynced  *atomic.Bool
	quit      chan struct{}
}

func NewBTCScanner(
	scannerCfg *config.BTCScannerConfig,
	logger *zap.Logger,
	btcClient btcclient.BTCClient,
	baseHeight uint64,
) (*BtcScanner, error) {
	confirmedBlocksChan := make(chan *vtypes.IndexedBlock, scannerCfg.BlockBufferSize)
	unconfirmedBlockCache, err := vtypes.NewBTCCache(scannerCfg.CacheSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create BTC cache for tail blocks: %w", err)
	}

	return &BtcScanner{
		logger:                logger.With(zap.String("module", "btcscanner")),
		btcClient:             btcClient,
		baseHeight:            baseHeight,
		k:                     scannerCfg.ConfirmationDepth,
		unconfirmedBlockCache: unconfirmedBlockCache,
		confirmedBlocksChan:   confirmedBlocksChan,
		isStarted:             atomic.NewBool(false),
		isSynced:              atomic.NewBool(false),
		quit:                  make(chan struct{}),
	}, nil
}

// Start starts the scanning process from curBTCHeight to tipHeight
func (bs *BtcScanner) Start() error {
	if bs.isStarted.Swap(true) {
		return fmt.Errorf("the BTC scanner is already started")
	}

	bs.logger.Info("starting the BTC scanner")

	// the bootstrapping should not block the main thread
	bs.wg.Add(1)
	go bs.Bootstrap()

	bs.btcClient.MustSubscribeBlocks()

	bs.isStarted.Store(true)
	bs.logger.Info("the BTC scanner is started")

	// start handling new blocks
	bs.wg.Add(1)
	go bs.blockEventLoop()

	return nil
}

// Bootstrap syncs with BTC by getting the confirmed blocks and the caching the unconfirmed blocks
func (bs *BtcScanner) Bootstrap() {
	var (
		firstUnconfirmedHeight uint64
		confirmedBlock         *vtypes.IndexedBlock
		err                    error
	)

	if bs.isSynced.Load() {
		// the scanner is already synced
		return
	}
	defer bs.isSynced.Store(true)

	if bs.confirmedTipBlock != nil {
		firstUnconfirmedHeight = uint64(bs.confirmedTipBlock.Height + 1)
	} else {
		firstUnconfirmedHeight = bs.baseHeight
	}

	bs.logger.Info("the bootstrapping starts", zap.Uint64("start height", firstUnconfirmedHeight))

	// clear all the blocks in the cache to avoid forks
	bs.unconfirmedBlockCache.RemoveAll()

	_, bestHeight, err := bs.btcClient.GetBestBlock()
	if err != nil {
		panic(fmt.Errorf("cannot get the best BTC block"))
	}

	bestConfirmedHeight := bestHeight - bs.k
	// process confirmed blocks
	for i := firstUnconfirmedHeight; i <= bestConfirmedHeight; i++ {
		ib, _, err := bs.btcClient.GetBlockByHeight(i)
		if err != nil {
			panic(err)
		}

		// this is a confirmed block
		confirmedBlock = ib

		// if the scanner was bootstrapped before, the new confirmed canonical chain must connect to the previous one
		if bs.confirmedTipBlock != nil {
			confirmedTipHash := bs.confirmedTipBlock.BlockHash()
			if !confirmedTipHash.IsEqual(&confirmedBlock.Header.PrevBlock) {
				panic("invalid canonical chain")
			}
		}

		bs.sendConfirmedBlocksToChan([]*vtypes.IndexedBlock{confirmedBlock})
	}

	// add unconfirmed blocks into the cache
	for i := bestConfirmedHeight + 1; i <= bestHeight; i++ {
		ib, _, err := bs.btcClient.GetBlockByHeight(i)
		if err != nil {
			panic(err)
		}

		// the unconfirmed blocks must follow the canonical chain
		tipCache := bs.unconfirmedBlockCache.Tip()
		if tipCache != nil {
			tipHash := tipCache.BlockHash()
			if !tipHash.IsEqual(&ib.Header.PrevBlock) {
				panic("invalid canonical chain")
			}
		}

		bs.unconfirmedBlockCache.Add(ib)
	}

	bs.logger.Info("bootstrapping is finished", zap.Uint64("best confirmed height", bestConfirmedHeight))
}

func (bs *BtcScanner) sendConfirmedBlocksToChan(blocks []*vtypes.IndexedBlock) {
	for i := 0; i < len(blocks); i++ {
		bs.confirmedBlocksChan <- blocks[i]
	}
	bs.confirmedTipBlock = blocks[len(blocks)-1]
}

func (bs *BtcScanner) ConfirmedBlocksChan() chan *vtypes.IndexedBlock {
	return bs.confirmedBlocksChan
}

func (bs *BtcScanner) Stop() error {
	if !bs.isStarted.Swap(false) {
		return nil
	}

	bs.btcClient.Stop()

	close(bs.quit)
	bs.wg.Wait()

	bs.logger.Info("the staking indexer is successfully stopped")

	return nil
}

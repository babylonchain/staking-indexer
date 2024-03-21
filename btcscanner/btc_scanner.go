package btcscanner

import (
	"fmt"
	"sync"
	"time"

	"github.com/babylonchain/vigilante/btcclient"
	vtypes "github.com/babylonchain/vigilante/types"
	notifier "github.com/lightningnetwork/lnd/chainntnfs"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
)

type BtcScanner struct {
	logger *zap.Logger

	// connect to BTC node
	btcClient   btcclient.BTCClient
	btcNotifier notifier.ChainNotifier

	cfg *config.BTCScannerConfig

	// the last confirmed BTC height
	lastConfirmedHeight uint64

	// communicate with the consumer
	confirmedBlocksChan chan *vtypes.IndexedBlock

	wg        sync.WaitGroup
	isStarted *atomic.Bool
	quit      chan struct{}
}

func NewBTCScanner(
	scannerCfg *config.BTCScannerConfig,
	logger *zap.Logger,
	btcClient btcclient.BTCClient,
	btcNotifier notifier.ChainNotifier,
	lastConfirmedHeight uint64,
) (*BtcScanner, error) {

	return &BtcScanner{
		logger:              logger.With(zap.String("module", "btcscanner")),
		btcClient:           btcClient,
		btcNotifier:         btcNotifier,
		cfg:                 scannerCfg,
		lastConfirmedHeight: lastConfirmedHeight,
		confirmedBlocksChan: make(chan *vtypes.IndexedBlock),
		isStarted:           atomic.NewBool(false),
		quit:                make(chan struct{}),
	}, nil
}

// Start starts the scanning process from the last confirmed height + 1
func (bs *BtcScanner) Start() error {
	if bs.isStarted.Swap(true) {
		return fmt.Errorf("the BTC scanner is already started")
	}

	bs.logger.Info("starting the BTC scanner")

	blockEventNotifier, err := bs.btcNotifier.RegisterBlockEpochNtfn(nil)
	if err != nil {
		return fmt.Errorf("failed to register block event from BTC notifier: %w", err)
	}

	bs.logger.Info("BTC notifier registered")

	if err := bs.bootstrap(blockEventNotifier); err != nil {
		return fmt.Errorf("failed to bootstrap: %w", err)
	}

	bs.wg.Add(1)
	go bs.pollBlocksLoop(blockEventNotifier)

	bs.logger.Info("the BTC scanner is started")

	return nil
}

func (bs *BtcScanner) bootstrap(blockEventNotifier *notifier.BlockEpochEvent) error {
	var tipHeight uint64

	bs.logger.Info("start bootstrapping",
		zap.Uint64("last_confirmed_height", bs.lastConfirmedHeight))

	select {
	case block := <-blockEventNotifier.Epochs:
		tipHeight = uint64(block.Height)
		bs.logger.Info("initial BTC best block", zap.Uint64("height", tipHeight))
	case <-bs.quit:
		return fmt.Errorf("quit before finishing bootstrapping")
	}

	err := bs.pollConfirmedBlocks(tipHeight)
	if err != nil {
		return fmt.Errorf("failed to poll confirmed blocks: %w", err)
	}

	bs.logger.Info("finished bootstrapping",
		zap.Uint64("last_confirmed_height", bs.lastConfirmedHeight))

	return err
}

// pollBlocksLoop polls confirmed blocks upon new block event and timeout
func (bs *BtcScanner) pollBlocksLoop(blockNotifier *notifier.BlockEpochEvent) {
	defer bs.wg.Done()
	defer blockNotifier.Cancel()

	pollBlocksTicker := time.NewTicker(bs.cfg.PollingInterval)

	for {
		select {
		case blockEpoch, ok := <-blockNotifier.Epochs:
			if !ok {
				bs.logger.Error("block event channel is closed")
				return
			}
			pollBlocksTicker.Reset(bs.cfg.PollingInterval)

			newBlock := blockEpoch
			bs.logger.Info("received a new best btc block",
				zap.Int32("height", newBlock.Height))

			err := bs.pollConfirmedBlocks(uint64(newBlock.Height))
			if err != nil {
				bs.logger.Error("failed to poll confirmed blocks", zap.Error(err))
				continue
			}

		case <-pollBlocksTicker.C:
			_, tipHeight, err := bs.btcClient.GetBestBlock()
			if err != nil {
				bs.logger.Error("failed to get the best block", zap.Error(err))
				continue
			}
			bs.logger.Info("polling confirmed blocks",
				zap.Uint64("tip_height", tipHeight))
			err = bs.pollConfirmedBlocks(tipHeight)
			if err != nil {
				bs.logger.Error("failed to poll confirmed blocks", zap.Error(err))
				continue
			}

		case <-bs.quit:
			bs.logger.Info("closing the block event loop")
			return
		}
	}
}

func (bs *BtcScanner) pollConfirmedBlocks(tipHeight uint64) error {
	k := bs.cfg.ConfirmationDepth

	if bs.lastConfirmedHeight+k >= tipHeight {
		bs.logger.Info("no confirmed blocks to poll",
			zap.Uint64("last_confirmed_height", bs.lastConfirmedHeight),
			zap.Uint64("current_tip_height", tipHeight))

		return nil
	}

	// start to poll confirmed blocks from the last confirmed height + 1
	// until tipHeight - k
	for i := bs.lastConfirmedHeight + 1; i+k <= tipHeight; i++ {
		block, _, err := bs.btcClient.GetBlockByHeight(i)
		if err != nil {
			return fmt.Errorf("failed to get block at height %d: %w", i, err)
		}

		bs.logger.Info("polled block",
			zap.Int32("height", block.Height))

		bs.sendConfirmedBlockToChan(block)
	}

	return nil
}

func (bs *BtcScanner) sendConfirmedBlockToChan(block *vtypes.IndexedBlock) {
	bs.confirmedBlocksChan <- block
	// TODO persist the state
	bs.lastConfirmedHeight = uint64(block.Height)
}

func (bs *BtcScanner) ConfirmedBlocksChan() chan *vtypes.IndexedBlock {
	return bs.confirmedBlocksChan
}

func (bs *BtcScanner) LastConfirmedHeight() uint64 {
	return bs.lastConfirmedHeight
}

func (bs *BtcScanner) Stop() error {
	if !bs.isStarted.Swap(false) {
		return nil
	}

	close(bs.quit)
	bs.wg.Wait()

	bs.logger.Info("the BTC scanner is successfully stopped")

	return nil
}

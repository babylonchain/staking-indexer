package btcscanner

import (
	"fmt"
	"sync"

	notifier "github.com/lightningnetwork/lnd/chainntnfs"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/types"
)

type BtcScanner interface {
	Start(startHeight uint64) error
	ConfirmedBlocksChan() chan *types.IndexedBlock
	LastConfirmedHeight() uint64
	GetRangeBlocks(fromHeight, targetHeight uint64) ([]*types.IndexedBlock, error)
	CurrentTipHeight() uint64
	Stop() error
}

type BtcPoller struct {
	logger *zap.Logger

	// connect to BTC node
	btcClient   Client
	btcNotifier notifier.ChainNotifier

	paramsVersions *types.ParamsVersions

	// the last confirmed BTC height
	lastConfirmedHeight uint64
	// the current tip BTC height
	currentTipHeight uint64

	// communicate with the consumer
	confirmedBlocksChan chan *types.IndexedBlock

	wg        sync.WaitGroup
	isStarted *atomic.Bool
	quit      chan struct{}
}

func NewBTCScanner(
	paramsVersions *types.ParamsVersions,
	logger *zap.Logger,
	btcClient Client,
	btcNotifier notifier.ChainNotifier,
) (*BtcPoller, error) {
	return &BtcPoller{
		logger:              logger.With(zap.String("module", "btcscanner")),
		btcClient:           btcClient,
		btcNotifier:         btcNotifier,
		paramsVersions:      paramsVersions,
		confirmedBlocksChan: make(chan *types.IndexedBlock),
		isStarted:           atomic.NewBool(false),
		quit:                make(chan struct{}),
	}, nil
}

// Start starts the scanning process from the last confirmed height + 1
func (bs *BtcPoller) Start(startHeight uint64) error {
	if bs.isStarted.Swap(true) {
		return fmt.Errorf("the BTC scanner is already started")
	}

	if startHeight == 0 {
		return fmt.Errorf("start height should be positive")
	}

	bs.lastConfirmedHeight = startHeight - 1

	bs.logger.Info("starting the BTC scanner", zap.Uint64("start_height", startHeight))

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

func (bs *BtcPoller) bootstrap(blockEventNotifier *notifier.BlockEpochEvent) error {
	var tipHeight uint64

	bs.logger.Info("start bootstrapping",
		zap.Uint64("last_confirmed_height", bs.lastConfirmedHeight))

	select {
	case block := <-blockEventNotifier.Epochs:
		tipHeight = uint64(block.Height)
		bs.currentTipHeight = tipHeight
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
func (bs *BtcPoller) pollBlocksLoop(blockNotifier *notifier.BlockEpochEvent) {
	defer bs.wg.Done()
	defer blockNotifier.Cancel()

	for {
		select {
		case blockEpoch, ok := <-blockNotifier.Epochs:
			if !ok {
				bs.logger.Error("block event channel is closed")
				return
			}

			tipHeight := uint64(blockEpoch.Height)
			bs.currentTipHeight = tipHeight
			bs.logger.Info("received a new best btc block",
				zap.Uint64("height", tipHeight))

			err := bs.pollConfirmedBlocks(tipHeight)
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

func (bs *BtcPoller) pollConfirmedBlocks(tipHeight uint64) error {
	p, err := bs.paramsVersions.GetParamsForBTCHeight(int32(tipHeight))
	if err != nil {
		return fmt.Errorf("failed to get params: %w", err)
	}
	k := uint64(p.ConfirmationDepth)

	if bs.lastConfirmedHeight+k >= tipHeight {
		bs.logger.Info("no confirmed blocks to poll",
			zap.Uint64("last_confirmed_height", bs.lastConfirmedHeight),
			zap.Uint64("current_tip_height", tipHeight))

		return nil
	}

	// start to poll confirmed blocks from the last confirmed height + 1
	// until tipHeight - k
	for i := bs.lastConfirmedHeight + 1; i+k <= tipHeight; i++ {
		block, err := bs.btcClient.GetBlockByHeight(i)
		if err != nil {
			return fmt.Errorf("failed to get block at height %d: %w", i, err)
		}

		bs.logger.Info("polled block",
			zap.Int32("height", block.Height))

		bs.sendConfirmedBlockToChan(block)
	}

	return nil
}

func (bs *BtcPoller) sendConfirmedBlockToChan(block *types.IndexedBlock) {
	bs.confirmedBlocksChan <- block
	bs.lastConfirmedHeight = uint64(block.Height)
}

func (bs *BtcPoller) ConfirmedBlocksChan() chan *types.IndexedBlock {
	return bs.confirmedBlocksChan
}

func (bs *BtcPoller) GetRangeBlocks(fromHeight, targetHeight uint64) ([]*types.IndexedBlock, error) {
	if fromHeight > targetHeight {
		return nil, fmt.Errorf("the from height %d should not be higher than the target height %d", fromHeight, targetHeight)
	}

	blocks := make([]*types.IndexedBlock, 0, targetHeight-fromHeight+1)
	for h := fromHeight; h <= targetHeight; h++ {
		b, err := bs.btcClient.GetBlockByHeight(h)
		if err != nil {
			return nil, fmt.Errorf("failed to get block at height %d: %w", h, err)
		}

		blocks = append(blocks, b)
	}

	return blocks, nil
}

func (bs *BtcPoller) CurrentTipHeight() uint64 {
	return bs.currentTipHeight
}

func (bs *BtcPoller) LastConfirmedHeight() uint64 {
	return bs.lastConfirmedHeight
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

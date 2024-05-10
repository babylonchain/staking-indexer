package btcscanner

import (
	"errors"
	"fmt"

	notifier "github.com/lightningnetwork/lnd/chainntnfs"
	"go.uber.org/zap"
)

// blockEventLoop handles connected and disconnected blocks from the BTC client.
func (bs *BtcPoller) blockEventLoop(blockNotifier *notifier.BlockEpochEvent) {
	defer bs.wg.Done()
	defer blockNotifier.Cancel()

	for {
		select {
		case blockEpoch, ok := <-blockNotifier.Epochs:
			newBlock := blockEpoch
			if !ok {
				bs.logger.Error("Block event channel is closed")
				return // channel closed
			}
			bs.logger.Debug("received new best btc block",
				zap.Int32("height", newBlock.Height))
			err := bs.handleNewBlock(newBlock)
			if err != nil {
				bs.logger.Warn("failed to handle a new block, need bootstrapping",
					zap.Int32("height", newBlock.Height),
					zap.Error(err))
				if bs.isSynced.Swap(false) {
					err := bs.Bootstrap(bs.LastConfirmedHeight() + 1)
					if err != nil {
						bs.logger.Error("failed to bootstrap",
							zap.Error(err))
					}
				}
			}
		case <-bs.quit:
			bs.logger.Info("closing the block event loop")
			return
		}
	}
}

// handleNewBlock handles connected blocks from the BTC client
// if new confirmed blocks are found, send them through the channel
func (bs *BtcPoller) handleNewBlock(blockEpoch *notifier.BlockEpoch) error {
	if !bs.isSynced.Load() {
		return errors.New("the btc scanner is not synced")
	}

	// get the block from hash
	ib, err := bs.btcClient.GetBlockByHeight(uint64(blockEpoch.Height))
	if err != nil {
		// TODO add retry
		return fmt.Errorf("failed to get block at height %d: %w", blockEpoch.Height, err)
	}

	// get cache tip
	cacheTip := bs.unconfirmedBlockCache.Tip()
	if cacheTip == nil {
		return fmt.Errorf("no unconfirmed blocks found")
	}

	parentHash := ib.Header.PrevBlock

	// if the parent of the block is not the tip of the cache, then the cache is not up-to-date
	if parentHash != cacheTip.BlockHash() {
		return fmt.Errorf("cache is not up-to-date")
	}

	// otherwise, add the block to the cache
	bs.unconfirmedBlockCache.Add(ib)

	params, err := bs.paramsVersions.GetParamsForBTCHeight(blockEpoch.Height)
	if err != nil {
		return fmt.Errorf("failed to get parameters for height %d: %w", blockEpoch.Height, err)
	}

	// still unconfirmed
	if bs.unconfirmedBlockCache.Size() <= uint64(params.ConfirmationDepth) {
		return nil
	}

	confirmedBlocks := bs.unconfirmedBlockCache.TrimConfirmedBlocks(int(params.ConfirmationDepth))
	if confirmedBlocks == nil {
		return nil
	}

	confirmedTipHash := bs.confirmedTipBlock.BlockHash()
	if !confirmedTipHash.IsEqual(&confirmedBlocks[0].Header.PrevBlock) {
		return fmt.Errorf("invalid canonical chain")
	}

	bs.sendConfirmedBlocksToChan(confirmedBlocks)

	return nil
}

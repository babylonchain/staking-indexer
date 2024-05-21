package btcscanner

import (
	"fmt"

	notifier "github.com/lightningnetwork/lnd/chainntnfs"
	"go.uber.org/zap"
)

// blockEventLoop handles new blocks from the BTC client unpon new block event
// Note: in case of rollback, the blockNotifier will emit information about new
// best block for every new block after rollback
func (bs *BtcPoller) blockEventLoop() {
	defer bs.wg.Done()

	bestKnownBlock := bs.unconfirmedBlockCache.Tip()
	bestKnownBlockEpoch := new(notifier.BlockEpoch)

	if bestKnownBlock != nil {
		bestKnownBlockHash := bestKnownBlock.BlockHash()

		bestKnownBlockEpoch = &notifier.BlockEpoch{
			Hash:        &bestKnownBlockHash,
			Height:      bestKnownBlock.Height,
			BlockHeader: bestKnownBlock.Header,
		}
	}

	blockEventNotifier, err := bs.btcNotifier.RegisterBlockEpochNtfn(bestKnownBlockEpoch)
	defer blockEventNotifier.Cancel()
	if err != nil {
		panic(fmt.Errorf("failed to register BTC notifier"))
	}

	bs.logger.Info("BTC notifier registered")

	for {
		select {
		case blockEpoch, ok := <-blockEventNotifier.Epochs:
			newBlock := blockEpoch
			if !ok {
				bs.logger.Error("Block event channel is closed")
				return // channel closed
			}
			bs.logger.Debug("received new best btc block",
				zap.Int32("height", newBlock.Height))

			err := bs.handleNewBlock(newBlock)
			if err != nil {
				bs.logger.Debug("failed to handle a new block, need bootstrapping",
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

// handleNewBlock handles a new block by adding it in the unconfirmed
// block cache, and extracting confirmed blocks if there are any
// error will be returned if the new block is not in the same branch
// of the cache
func (bs *BtcPoller) handleNewBlock(blockEpoch *notifier.BlockEpoch) error {
	if !bs.isSynced.Load() {
		return fmt.Errorf("the btc scanner is not synced")
	}

	// get cache tip and check whether this block is expected
	cacheTip := bs.unconfirmedBlockCache.Tip()
	if cacheTip == nil {
		return fmt.Errorf("no unconfirmed blocks found")
	}

	if cacheTip.Height >= blockEpoch.Height {
		bs.logger.Debug("skip handling a low block",
			zap.Int32("block_height", blockEpoch.Height),
			zap.Int32("unconfirmed_tip_height", cacheTip.Height))
		return nil
	}

	if cacheTip.Height+1 < blockEpoch.Height {
		return fmt.Errorf("missing blocks, expected block height: %d, got: %d",
			cacheTip.Height+1, blockEpoch.Height)
	}

	// check whether the block connects to the cache tip
	parentHash := blockEpoch.BlockHeader.PrevBlock
	if parentHash != cacheTip.BlockHash() {
		return fmt.Errorf("the block's parent hash does not match the cache tip")
	}

	// get the block content by height
	ib, err := bs.btcClient.GetBlockByHeight(uint64(blockEpoch.Height))
	if err != nil {
		// TODO add retry
		return fmt.Errorf("failed to get block at height %d: %w", blockEpoch.Height, err)
	}
	if ib.BlockHash().String() != blockEpoch.Hash.String() {
		return fmt.Errorf("re-org happened at height %d", blockEpoch.Height)
	}

	// add the block to the cache
	if err := bs.unconfirmedBlockCache.Add(ib); err != nil {
		return fmt.Errorf("failed to add the block %d to cache: %w", ib.Height, err)
	}

	params, err := bs.paramsVersions.GetParamsForBTCHeight(blockEpoch.Height)
	if err != nil {
		return fmt.Errorf("failed to get parameters for height %d: %w", blockEpoch.Height, err)
	}

	// try to extract confirmed blocks
	confirmedBlocks := bs.unconfirmedBlockCache.TrimConfirmedBlocks(int(params.ConfirmationDepth) - 1)

	// send confirmed blocks to the consumer
	bs.sendConfirmedBlocksToChan(confirmedBlocks)

	// send tip unconfirmed block to the consumer
	bs.sendTipUnconfirmedBlockToChan()

	return nil
}

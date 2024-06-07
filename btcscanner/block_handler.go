package btcscanner

import (
	"fmt"

	notifier "github.com/lightningnetwork/lnd/chainntnfs"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/types"
)

// blockEventLoop handles new blocks from the BTC client upon new block event
// Note: in case of rollback, the blockNotifier will emit information about new
// best block for every new block after rollback
func (bs *BtcPoller) blockEventLoop(startHeight uint64) {
	defer bs.wg.Done()

	var (
		blockEventNotifier *notifier.BlockEpochEvent
		err                error
	)
	// register the notifier with the best known tip
	bestKnownBlock := bs.unconfirmedBlockCache.Tip()
	if bestKnownBlock != nil {
		bestKnownBlockHash := bestKnownBlock.BlockHash()
		bestKnownBlockEpoch := &notifier.BlockEpoch{
			Hash:        &bestKnownBlockHash,
			Height:      bestKnownBlock.Height,
			BlockHeader: bestKnownBlock.Header,
		}
		blockEventNotifier, err = bs.btcNotifier.RegisterBlockEpochNtfn(bestKnownBlockEpoch)
	} else {
		blockEventNotifier, err = bs.btcNotifier.RegisterBlockEpochNtfn(nil)
	}

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
					bootStrapHeight := startHeight
					lastConfirmedHeight := bs.LastConfirmedHeight()
					if lastConfirmedHeight != 0 {
						bootStrapHeight = lastConfirmedHeight + 1
					}

					err := bs.Bootstrap(bootStrapHeight)
					if err != nil {
						bs.logger.Error("failed to bootstrap",
							zap.Uint64("start_height", bootStrapHeight),
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
		return fmt.Errorf("failed to get block at height %d: %w", blockEpoch.Height, err)
	}
	if ib.BlockHash().String() != blockEpoch.Hash.String() {
		return fmt.Errorf("re-org happened at height %d", blockEpoch.Height)
	}

	// add the block to the cache
	if err := bs.unconfirmedBlockCache.Add(ib); err != nil {
		return fmt.Errorf("failed to add the block %d to cache: %w", ib.Height, err)
	}

	// try to extract confirmed blocks
	confirmedBlocks := bs.unconfirmedBlockCache.TrimConfirmedBlocks(int(bs.confirmationDepth) - 1)

	bs.commitChainUpdate(confirmedBlocks)

	return nil
}

func (bs *BtcPoller) commitChainUpdate(confirmedBlocks []*types.IndexedBlock) {
	if len(confirmedBlocks) != 0 {
		if bs.confirmedTipBlock != nil {
			confirmedTipHash := bs.confirmedTipBlock.BlockHash()
			if !confirmedTipHash.IsEqual(&confirmedBlocks[0].Header.PrevBlock) {
				// this indicates either programmatic error or the confirmation
				// depth is not large enough to cover re-orgs
				panic(fmt.Errorf("invalid canonical chain"))
			}
		}
		bs.confirmedTipBlock = confirmedBlocks[len(confirmedBlocks)-1]
	}

	chainUpdateInfo := &ChainUpdateInfo{
		ConfirmedBlocks:     confirmedBlocks,
		TipUnconfirmedBlock: bs.unconfirmedBlockCache.Tip(),
	}

	select {
	case bs.chainUpdateInfoChan <- chainUpdateInfo:
	case <-bs.quit:
	}
}

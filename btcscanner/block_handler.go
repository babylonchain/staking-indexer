package btcscanner

import (
	"errors"

	notifier "github.com/lightningnetwork/lnd/chainntnfs"
	"go.uber.org/zap"
)

// blockEventLoop handles connected and disconnected blocks from the BTC client.
func (bs *BtcScanner) blockEventLoop(blockNotifier *notifier.BlockEpochEvent) {
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
					bs.Bootstrap()
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
func (bs *BtcScanner) handleNewBlock(blockEpoch *notifier.BlockEpoch) error {
	if !bs.isSynced.Load() {
		return errors.New("the btc scanner is not synced")
	}

	// get the block from hash
	ib, _, err := bs.btcClient.GetBlockByHash(blockEpoch.Hash)
	if err != nil {
		// failing to request the block, which means a bug
		// TODO should use retry to avoid panic
		panic(err)
	}

	// get cache tip
	cacheTip := bs.unconfirmedBlockCache.Tip()
	if cacheTip == nil {
		return errors.New("no unconfirmed blocks found")
	}

	parentHash := ib.Header.PrevBlock

	// if the parent of the block is not the tip of the cache, then the cache is not up-to-date
	if parentHash != cacheTip.BlockHash() {
		return errors.New("cache is not up-to-date")
	}

	// otherwise, add the block to the cache
	bs.unconfirmedBlockCache.Add(ib)

	// still unconfirmed
	if bs.unconfirmedBlockCache.Size() <= bs.k {
		return nil
	}

	confirmedBlocks := bs.unconfirmedBlockCache.TrimConfirmedBlocks(int(bs.k))
	if confirmedBlocks == nil {
		return nil
	}

	confirmedTipHash := bs.confirmedTipBlock.BlockHash()
	if !confirmedTipHash.IsEqual(&confirmedBlocks[0].Header.PrevBlock) {
		panic("invalid canonical chain")
	}

	bs.sendConfirmedBlocksToChan(confirmedBlocks)

	return nil
}

package btcscanner

import (
	"errors"
	"fmt"

	"github.com/babylonchain/vigilante/types"
	"go.uber.org/zap"
)

// blockEventLoop handles connected and disconnected blocks from the BTC client.
func (bs *BtcScanner) blockEventLoop() {
	defer bs.wg.Done()

	for {
		select {
		case event, open := <-bs.btcClient.BlockEventChan():
			if !open {
				bs.logger.Error("Block event channel is closed")
				return // channel closed
			}
			if event.EventType == types.BlockConnected {
				err := bs.handleConnectedBlocks(event)
				if err != nil {
					bs.logger.Warn("failed to handle a connected block, need bootstrapping",
						zap.Int32("height", event.Height),
						zap.Error(err))
					if bs.isSynced.Swap(false) {
						bs.Bootstrap()
					}
				}
			} else if event.EventType == types.BlockDisconnected {
				err := bs.handleDisconnectedBlocks(event)
				if err != nil {
					bs.logger.Warn("failed to handle a disconnected block, need bootstrapping",
						zap.Int32("height", event.Height),
						zap.Error(err))
					if bs.isSynced.Swap(false) {
						bs.Bootstrap()
					}
				}
			}
		case <-bs.quit:
			bs.logger.Info("closing the block event loop")
			return
		}
	}
}

// handleConnectedBlocks handles connected blocks from the BTC client
// if new confirmed blocks are found, send them through the channel
func (bs *BtcScanner) handleConnectedBlocks(event *types.BlockEvent) error {
	if !bs.isSynced.Load() {
		return errors.New("the btc scanner is not synced")
	}

	// get the block from hash
	blockHash := event.Header.BlockHash()
	ib, _, err := bs.btcClient.GetBlockByHash(&blockHash)
	if err != nil {
		// failing to request the block, which means a bug
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

// handleDisconnectedBlocks handles disconnected blocks from the BTC client.
func (bs *BtcScanner) handleDisconnectedBlocks(event *types.BlockEvent) error {
	// get cache tip
	cacheTip := bs.unconfirmedBlockCache.Tip()
	if cacheTip == nil {
		return errors.New("cache is empty")
	}

	// if the block to be disconnected is not the tip of the cache, then the cache is not up-to-date,
	if event.Header.BlockHash() != cacheTip.BlockHash() {
		return errors.New("cache is out-of-sync")
	}

	// otherwise, remove the block from the cache
	if err := bs.unconfirmedBlockCache.RemoveLast(); err != nil {
		return fmt.Errorf("failed to remove last block from cache: %v", err)
	}

	return nil
}

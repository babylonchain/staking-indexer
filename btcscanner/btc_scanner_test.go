package btcscanner_test

import (
	"math/rand"
	"sync"
	"testing"

	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	"github.com/golang/mock/gomock"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"github.com/lightningnetwork/lnd/lntest/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/btcscanner"
	"github.com/babylonchain/staking-indexer/testutils/datagen"
	"github.com/babylonchain/staking-indexer/testutils/mocks"
	"github.com/babylonchain/staking-indexer/types"
)

// FuzzBootstrap tests happy path of bootstrapping
func FuzzBootstrap(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 1000)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		versionedParams := datagen.GenerateGlobalParamsVersions(r, t)
		k := uint64(versionedParams.Versions[0].ConfirmationDepth)
		startHeight := versionedParams.Versions[0].ActivationHeight
		// Generate a random number of blocks
		numBlocks := bbndatagen.RandomIntOtherThan(r, 0, 200) + 1
		chainIndexedBlocks := datagen.GetRandomIndexedBlocks(r, startHeight, numBlocks)
		bestHeight := chainIndexedBlocks[len(chainIndexedBlocks)-1].Height

		ctl := gomock.NewController(t)
		mockBtcClient := mocks.NewMockClient(ctl)
		mockBtcClient.EXPECT().GetTipHeight().Return(uint64(bestHeight), nil).AnyTimes()

		confirmedBlocks := make([]*types.IndexedBlock, 0)
		if numBlocks >= k-1 {
			confirmedBlocks = chainIndexedBlocks[:numBlocks-k+1]
		}
		for i := 0; i < int(numBlocks); i++ {
			mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(chainIndexedBlocks[i].Height))).
				Return(chainIndexedBlocks[i], nil).AnyTimes()
		}

		btcScanner, err := btcscanner.NewBTCScanner(uint16(k), zap.NewNop(), mockBtcClient, &mock.ChainNotifier{})
		require.NoError(t, err)

		var wg sync.WaitGroup

		numBatches := len(confirmedBlocks)/btcscanner.ConfirmedBlockBatchSize + 1

		wg.Add(1)
		go func() {
			defer wg.Done()
			confirmedBlockIndex := 0
			for i := 0; i < numBatches; i++ {
				updateInfo := <-btcScanner.ChainUpdateInfoChan()
				for _, b := range updateInfo.ConfirmedBlocks {
					require.Equal(t, confirmedBlocks[confirmedBlockIndex].Height, b.Height)
					require.Equal(t, confirmedBlocks[confirmedBlockIndex].BlockHash(), b.BlockHash())
					confirmedBlockIndex++
				}

				// this is the case where bootstrap to a small height and
				// no confirmed blocks found
				if numBatches == 1 && len(updateInfo.ConfirmedBlocks) == 0 {
					require.Equal(t, chainIndexedBlocks, updateInfo.UnconfirmedBlocks)
				} else if len(updateInfo.ConfirmedBlocks) != 0 {
					require.Equal(t, int(k)-1, len(updateInfo.UnconfirmedBlocks))
					lastConfirmedHeight := updateInfo.ConfirmedBlocks[len(updateInfo.ConfirmedBlocks)-1].Height
					require.Equal(t, lastConfirmedHeight+1, updateInfo.UnconfirmedBlocks[0].Height)
				}
			}
		}()

		err = btcScanner.Bootstrap(startHeight)
		require.NoError(t, err)

		wg.Wait()
	})
}

// FuzzHandleNewBlock tests (1) happy path of handling an incoming block,
// and (2) errors when the incoming block is not expected
func FuzzHandleNewBlock(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 100)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		versionedParams := datagen.GenerateGlobalParamsVersions(r, t)
		k := uint64(versionedParams.Versions[0].ConfirmationDepth)
		startHeight := versionedParams.Versions[0].ActivationHeight

		// Generate a random number of blocks
		numBlocks := bbndatagen.RandomIntOtherThan(r, 0, 50) + k // make sure we have at least k+1 entry
		initialChain := datagen.GetRandomIndexedBlocks(r, startHeight, numBlocks)
		bestHeight := initialChain[len(initialChain)-1].Height
		bestBlockHash := initialChain[len(initialChain)-1].BlockHash()

		numBlocks1 := bbndatagen.RandomIntOtherThan(r, 0, 50)
		firstChainedIndexedBlocks := datagen.GetRandomIndexedBlocksFromHeight(r, numBlocks1, bestHeight, bestBlockHash)
		firstChainedBlockEpochs := indexedBlocksToBlockEpochs(firstChainedIndexedBlocks)
		canonicalChain := append(initialChain, firstChainedIndexedBlocks...)

		ctl := gomock.NewController(t)
		mockBtcClient := mocks.NewMockClient(ctl)
		mockBtcClient.EXPECT().GetTipHeight().Return(uint64(bestHeight), nil).AnyTimes()
		for _, b := range canonicalChain {
			mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(b.Height))).
				Return(b, nil).AnyTimes()
		}

		numBlocks2 := bbndatagen.RandomIntOtherThan(r, 0, 50) + numBlocks1
		secondChainedIndexedBlocks := datagen.GetRandomIndexedBlocksFromHeight(r, numBlocks2, bestHeight, bestBlockHash)
		secondChainedBlockEpochs := indexedBlocksToBlockEpochs(secondChainedIndexedBlocks)

		btcScanner, err := btcscanner.NewBTCScanner(uint16(k), zap.NewNop(), mockBtcClient, &mock.ChainNotifier{})
		require.NoError(t, err)

		// receive confirmed blocks
		go func() {
			for {
				<-btcScanner.ChainUpdateInfoChan()
			}
		}()

		err = btcScanner.Start(startHeight, startHeight)
		require.NoError(t, err)
		defer func() {
			err := btcScanner.Stop()
			require.NoError(t, err)
		}()

		for _, b := range firstChainedBlockEpochs {
			err := btcScanner.HandleNewBlock(b)
			require.NoError(t, err)
		}

		for i, b := range secondChainedBlockEpochs {
			err := btcScanner.HandleNewBlock(b)
			if i <= len(firstChainedBlockEpochs)-1 {
				require.NoError(t, err)
			} else if i == len(firstChainedBlockEpochs) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "the block's parent hash does not match the cache tip")
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), "missing blocks")
			}
		}
	})
}

// FuzzBootstrapMajorReorg tests the case when a major reorg is happening
// this should cause panic
func FuzzBootstrapMajorReorg(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 100)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		versionedParams := datagen.GenerateGlobalParamsVersions(r, t)
		k := uint64(10)
		startHeight := versionedParams.Versions[0].ActivationHeight
		// Generate a random number of blocks
		numBlocks := bbndatagen.RandomIntOtherThan(r, 0, 50) + k // make sure we have at least k+1 entry
		chainIndexedBlocks := datagen.GetRandomIndexedBlocks(r, startHeight, numBlocks)
		bestHeight := chainIndexedBlocks[len(chainIndexedBlocks)-1].Height
		confirmedBlocks := chainIndexedBlocks[:numBlocks-k+1]
		lastConfirmedBlock := confirmedBlocks[len(confirmedBlocks)-1]

		// major reorg chain is created from the last confirmed height but does not point
		// to the last confirmed block
		secondChain := datagen.GetRandomIndexedBlocks(r, uint64(lastConfirmedBlock.Height+1), numBlocks)
		secondBestHeight := secondChain[len(secondChain)-1].Height

		ctl := gomock.NewController(t)
		mockBtcClient := mocks.NewMockClient(ctl)
		tipHeightCall1 := mockBtcClient.EXPECT().GetTipHeight().Return(uint64(bestHeight), nil)
		mockBtcClient.EXPECT().GetTipHeight().Return(uint64(secondBestHeight), nil).After(tipHeightCall1)
		for i := chainIndexedBlocks[0].Height; i <= secondChain[len(secondChain)-1].Height; i++ {
			if i < secondChain[0].Height {
				firstBlock := chainIndexedBlocks[int(i-chainIndexedBlocks[0].Height)]
				mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(i))).
					Return(firstBlock, nil)
			} else if i <= bestHeight {
				firstBlock := chainIndexedBlocks[int(i-chainIndexedBlocks[0].Height)]
				c1 := mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(i))).
					Return(firstBlock, nil)
				secondBlock := secondChain[int(i-secondChain[0].Height)]
				mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(i))).
					Return(secondBlock, nil).After(c1)
			} else {
				secondBlock := secondChain[int(i-secondChain[0].Height)]
				mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(i))).
					Return(secondBlock, nil)
			}
		}

		btcScanner, err := btcscanner.NewBTCScanner(uint16(k), zap.NewNop(), mockBtcClient, &mock.ChainNotifier{})
		require.NoError(t, err)

		// receive confirmed blocks
		go func() {
			for {
				<-btcScanner.ChainUpdateInfoChan()
			}
		}()

		err = btcScanner.Bootstrap(startHeight)
		require.NoError(t, err)
		require.Panics(t, func() {
			_ = btcScanner.Bootstrap(uint64(lastConfirmedBlock.Height) + 1)
		})
	})
}

func indexedBlocksToBlockEpochs(ibs []*types.IndexedBlock) []*chainntnfs.BlockEpoch {
	blockEpochs := make([]*chainntnfs.BlockEpoch, 0)
	for _, ib := range ibs {
		blockHash := ib.BlockHash()
		be := &chainntnfs.BlockEpoch{
			Hash:        &blockHash,
			Height:      ib.Height,
			BlockHeader: ib.Header,
		}
		blockEpochs = append(blockEpochs, be)
	}

	return blockEpochs
}

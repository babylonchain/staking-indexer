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

func FuzzPollConfirmedBlocks(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		versionedParams := datagen.GenerateGlobalParamsVersions(r, t)
		k := uint64(versionedParams.ParamsVersions[0].ConfirmationDepth)
		// Generate a random number of blocks
		numBlocks := bbndatagen.RandomIntOtherThan(r, 0, 50) + k // make sure we have at least k+1 entry
		chainIndexedBlocks := datagen.GetRandomIndexedBlocks(r, numBlocks)
		startHeight := chainIndexedBlocks[0].Height
		bestHeight := chainIndexedBlocks[len(chainIndexedBlocks)-1].Height
		bestBlockHash := chainIndexedBlocks[len(chainIndexedBlocks)-1].BlockHash()

		ctl := gomock.NewController(t)
		mockBtcClient := mocks.NewMockClient(ctl)
		mockBtcClient.EXPECT().GetTipHeight().Return(uint64(bestHeight), nil).AnyTimes()
		confirmedBlocks := chainIndexedBlocks[:numBlocks-k+1]
		for i := 0; i < int(numBlocks); i++ {
			mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(chainIndexedBlocks[i].Height))).
				Return(chainIndexedBlocks[i], nil).AnyTimes()
		}

		epochChan := make(chan *chainntnfs.BlockEpoch, 1)
		bestEpoch := &chainntnfs.BlockEpoch{Height: bestHeight, Hash: &bestBlockHash}
		epochChan <- bestEpoch
		mockBtcNotifier := &mock.ChainNotifier{
			EpochChan: epochChan,
			SpendChan: make(chan *chainntnfs.SpendDetail),
			ConfChan:  make(chan *chainntnfs.TxConfirmation),
		}

		btcScanner, err := btcscanner.NewBTCScanner(versionedParams, zap.NewNop(), mockBtcClient, mockBtcNotifier)
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < len(confirmedBlocks); i++ {
				b := <-btcScanner.ConfirmedBlocksChan()
				require.Equal(t, confirmedBlocks[i].BlockHash(), b.BlockHash())
			}
		}()
		err = btcScanner.Start(uint64(startHeight))
		require.NoError(t, err)
		defer func() {
			err := btcScanner.Stop()
			require.NoError(t, err)
		}()

		wg.Wait()
		require.Equal(t, uint64(confirmedBlocks[len(confirmedBlocks)-1].Height), btcScanner.LastConfirmedHeight())
	})
}

func FuzzReorgBlocks(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		versionedParams := datagen.GenerateGlobalParamsVersions(r, t)
		k := uint64(versionedParams.ParamsVersions[0].ConfirmationDepth)

		// Generate a common prefix with random number of blocks
		commonPrefixNum := bbndatagen.RandomIntOtherThan(r, 0, 10) + k
		commonPrefixChain := datagen.GetRandomIndexedBlocks(r, commonPrefixNum)
		commonParent := commonPrefixChain[len(commonPrefixChain)-1]
		initialChainNum := bbndatagen.RandomIntOtherThan(r, 0, int(k)) // length shorter than k
		initialChain := datagen.GetRandomIndexedBlocksFromHeight(r, initialChainNum, commonParent.Height, commonParent.BlockHash())
		reorgChainNum := bbndatagen.RandomIntOtherThan(r, 0, 10) + k
		reorgChain := datagen.GetRandomIndexedBlocksFromHeight(r, reorgChainNum, commonParent.Height, commonParent.BlockHash())

		canonicalChain := make([]*types.IndexedBlock, 0)
		canonicalChain = append(canonicalChain, commonPrefixChain...)
		canonicalChain = append(canonicalChain, reorgChain...)
		confirmedBlocks := canonicalChain[:len(canonicalChain)-int(k)+1]
		epochChan := make(chan *chainntnfs.BlockEpoch, len(reorgChain))
		for _, b := range reorgChain {
			blockHash := b.BlockHash()
			blockEpoch := &chainntnfs.BlockEpoch{
				Hash:        &blockHash,
				Height:      b.Height,
				BlockHeader: b.Header,
			}
			epochChan <- blockEpoch
		}

		ctl := gomock.NewController(t)
		mockBtcClient := mocks.NewMockClient(ctl)
		for _, b := range commonPrefixChain {
			mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(b.Height))).
				Return(b, nil).AnyTimes()
		}
		for _, b := range initialChain {
			mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(b.Height))).
				Return(b, nil)
		}
		for _, b := range reorgChain {
			mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(b.Height))).
				Return(b, nil)
		}
		gomock.InOrder(
			mockBtcClient.EXPECT().GetTipHeight().Times(1).Return(uint64(initialChain[len(initialChain)-1].Height), nil),
			mockBtcClient.EXPECT().GetTipHeight().Times(1).Return(uint64(reorgChain[len(initialChain)-1].Height), nil),
		)

		mockBtcNotifier := &mock.ChainNotifier{
			EpochChan: epochChan,
			SpendChan: make(chan *chainntnfs.SpendDetail),
			ConfChan:  make(chan *chainntnfs.TxConfirmation),
		}

		btcScanner, err := btcscanner.NewBTCScanner(versionedParams, zap.NewNop(), mockBtcClient, mockBtcNotifier)
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < len(confirmedBlocks); i++ {
				b := <-btcScanner.ConfirmedBlocksChan()
				require.Equal(t, confirmedBlocks[i].BlockHash(), b.BlockHash())
			}
		}()
		err = btcScanner.Start(uint64(commonPrefixChain[0].Height))
		require.NoError(t, err)
		defer func() {
			err := btcScanner.Stop()
			require.NoError(t, err)
		}()

		wg.Wait()
		require.Equal(t, uint64(confirmedBlocks[len(confirmedBlocks)-1].Height), btcScanner.LastConfirmedHeight())
	})
}

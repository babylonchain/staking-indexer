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

		ctl := gomock.NewController(t)
		mockBtcClient := mocks.NewMockClient(ctl)
		mockBtcClient.EXPECT().GetTipHeight().Return(uint64(bestHeight), nil)
		confirmedBlocks := chainIndexedBlocks[:numBlocks-k]
		for i := 0; i < int(numBlocks); i++ {
			mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(chainIndexedBlocks[i].Height))).
				Return(chainIndexedBlocks[i], nil).AnyTimes()
		}

		epochChan := make(chan *chainntnfs.BlockEpoch, 1)
		bestEpoch := &chainntnfs.BlockEpoch{Height: bestHeight}
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

package btcscanner_test

import (
	"math/rand"
	"sync"
	"testing"

	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	"github.com/golang/mock/gomock"
	"github.com/lightningnetwork/lnd/lntest/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/btcscanner"
	"github.com/babylonchain/staking-indexer/testutils/datagen"
	"github.com/babylonchain/staking-indexer/testutils/mocks"
)

func FuzzPoller(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 100)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		versionedParams := datagen.GenerateGlobalParamsVersions(r, t)
		k := uint64(versionedParams.Versions[0].ConfirmationDepth)
		startHeight := versionedParams.Versions[0].ActivationHeight
		// Generate a random number of blocks
		numBlocks := bbndatagen.RandomIntOtherThan(r, 0, 50) + k // make sure we have at least k+1 entry
		chainIndexedBlocks := datagen.GetRandomIndexedBlocks(r, startHeight, numBlocks)
		bestHeight := chainIndexedBlocks[len(chainIndexedBlocks)-1].Height

		ctl := gomock.NewController(t)
		mockBtcClient := mocks.NewMockClient(ctl)
		mockBtcClient.EXPECT().GetTipHeight().Return(uint64(bestHeight), nil).AnyTimes()
		confirmedBlocks := chainIndexedBlocks[:numBlocks-k+1]
		for i := 0; i < int(numBlocks); i++ {
			mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(chainIndexedBlocks[i].Height))).
				Return(chainIndexedBlocks[i], nil).AnyTimes()
		}

		btcScanner, err := btcscanner.NewBTCScanner(uint16(k), zap.NewNop(), mockBtcClient, &mock.ChainNotifier{})
		require.NoError(t, err)

		var wg sync.WaitGroup

		// receive confirmed blocks
		wg.Add(1)
		go func() {
			defer wg.Done()
			updateInfo := <-btcScanner.ChainUpdateInfoChan()
			for i, b := range updateInfo.ConfirmedBlocks {
				require.Equal(t, confirmedBlocks[i].BlockHash(), b.BlockHash())
			}
			require.Equal(t, bestHeight, updateInfo.TipUnconfirmedBlock.Height)
		}()

		err = btcScanner.Start(startHeight, startHeight)
		require.NoError(t, err)
		defer func() {
			err := btcScanner.Stop()
			require.NoError(t, err)
		}()

		wg.Wait()
	})
}

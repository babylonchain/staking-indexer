package btcscanner_test

import (
	"math/rand"
	"sync"
	"testing"

	"github.com/babylonchain/babylon/testutil/datagen"
	vdatagen "github.com/babylonchain/vigilante/testutil/datagen"
	"github.com/babylonchain/vigilante/testutil/mocks"
	"github.com/golang/mock/gomock"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"github.com/lightningnetwork/lnd/lntest/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/btcscanner"
	"github.com/babylonchain/staking-indexer/config"
)

func FuzzPollConfirmedBlocks(f *testing.F) {
	datagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		cfg := config.DefaultBTCScannerConfig()
		k := cfg.ConfirmationDepth
		// Generate a random number of blocks
		numBlocks := datagen.RandomIntOtherThan(r, 0, 50) + k // make sure we have at least k+1 entry
		chainIndexedBlocks := vdatagen.GetRandomIndexedBlocks(r, numBlocks)
		lastConfirmedHeight := chainIndexedBlocks[0].Height - 1
		bestHeight := chainIndexedBlocks[len(chainIndexedBlocks)-1].Height

		ctl := gomock.NewController(t)
		mockBtcClient := mocks.NewMockBTCClient(ctl)
		confirmedBlocks := chainIndexedBlocks[:numBlocks-k]
		for i := 0; i < int(numBlocks); i++ {
			mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(chainIndexedBlocks[i].Height))).
				Return(chainIndexedBlocks[i], nil, nil).AnyTimes()
		}

		epochChan := make(chan *chainntnfs.BlockEpoch, 1)
		bestEpoch := &chainntnfs.BlockEpoch{Height: bestHeight}
		epochChan <- bestEpoch
		mockBtcNotifier := &mock.ChainNotifier{
			EpochChan: epochChan,
			SpendChan: make(chan *chainntnfs.SpendDetail),
			ConfChan:  make(chan *chainntnfs.TxConfirmation),
		}

		btcScanner, err := btcscanner.NewBTCScanner(cfg, zap.NewNop(), mockBtcClient, mockBtcNotifier, uint64(lastConfirmedHeight))
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
		err = btcScanner.Start()
		require.NoError(t, err)
		defer func() {
			err := btcScanner.Stop()
			require.NoError(t, err)
		}()

		wg.Wait()
		require.Equal(t, uint64(confirmedBlocks[len(confirmedBlocks)-1].Height), btcScanner.LastConfirmedHeight())
	})
}

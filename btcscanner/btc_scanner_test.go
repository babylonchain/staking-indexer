package btcscanner_test

import (
	"math/rand"
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

func FuzzBootStrap(f *testing.F) {
	datagen.AddRandomSeedsToFuzzer(f, 100)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		k := uint64(6)
		// Generate a random number of blocks
		numBlocks := datagen.RandomIntOtherThan(r, 0, 50) + k // make sure we have at least k+1 entry
		chainIndexedBlocks := vdatagen.GetRandomIndexedBlocks(r, numBlocks)
		baseHeight := chainIndexedBlocks[0].Height
		bestHeight := chainIndexedBlocks[len(chainIndexedBlocks)-1].Height

		ctl := gomock.NewController(t)
		mockBtcClient := mocks.NewMockBTCClient(ctl)
		confirmedBlocks := chainIndexedBlocks[:numBlocks-k]
		mockBtcClient.EXPECT().MustSubscribeBlocks().Return().AnyTimes()
		mockBtcClient.EXPECT().GetBestBlock().Return(nil, uint64(bestHeight), nil)
		for i := 0; i < int(numBlocks); i++ {
			mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(chainIndexedBlocks[i].Height))).
				Return(chainIndexedBlocks[i], nil, nil).AnyTimes()
		}

		mockBtcNotifier := &mock.ChainNotifier{
			EpochChan: make(chan *chainntnfs.BlockEpoch),
			SpendChan: make(chan *chainntnfs.SpendDetail),
			ConfChan:  make(chan *chainntnfs.TxConfirmation),
		}

		cfg := config.DefaultBTCScannerConfig()
		btcScanner, err := btcscanner.NewBTCScanner(cfg, zap.NewNop(), mockBtcClient, mockBtcNotifier, uint64(baseHeight))
		require.NoError(t, err)

		go func() {
			for i := 0; i < len(confirmedBlocks); i++ {
				b := <-btcScanner.ConfirmedBlocksChan()
				require.Equal(t, confirmedBlocks[i].BlockHash(), b.BlockHash())
			}
		}()
		btcScanner.Bootstrap()
	})
}

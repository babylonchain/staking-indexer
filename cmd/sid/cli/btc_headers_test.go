package cli_test

import (
	"math/rand"
	"testing"

	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"

	"github.com/babylonchain/staking-indexer/cmd/sid/cli"
	"github.com/babylonchain/staking-indexer/testutils/datagen"
	"github.com/babylonchain/staking-indexer/testutils/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func FuzzBtcHeaders(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		// Generate a random number of blocks
		numBlocks := bbndatagen.RandomInt(r, 50) + 30

		chainIndexedBlocks := datagen.GetRandomIndexedBlocks(r, numBlocks)
		startHeight := uint64(chainIndexedBlocks[0].Height)
		endHeight := uint64(chainIndexedBlocks[len(chainIndexedBlocks)-1].Height)

		ctl := gomock.NewController(t)
		mockBtcClient := mocks.NewMockClient(ctl)

		for i := 0; i < int(numBlocks); i++ {
			idxBlock := chainIndexedBlocks[i]
			mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(uint64(idxBlock.Height))).
				Return(idxBlock, nil).AnyTimes()
		}

		infos, err := cli.BtcHeaderInfoList(mockBtcClient, startHeight, endHeight)
		require.NoError(t, err)
		require.EqualValues(t, len(infos), numBlocks)
	})
}

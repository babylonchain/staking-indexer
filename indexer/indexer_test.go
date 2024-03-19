package indexer_test

import (
	"encoding/hex"
	"math/rand"
	"path/filepath"
	"testing"

	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	vtypes "github.com/babylonchain/vigilante/types"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/indexer"
	"github.com/babylonchain/staking-indexer/testutils/datagen"
)

// FuzzIndexer tests the property that the indexer can correctly
// parse staking tx from confirmed blocks
func FuzzIndexer(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		homePath := filepath.Join(t.TempDir(), "indexer")
		cfg := config.DefaultConfigWithHome(homePath)

		confirmedBlockChan := make(chan *vtypes.IndexedBlock)
		stakingIndexer, err := indexer.NewStakingIndexer(cfg, zap.NewNop(), confirmedBlockChan)
		require.NoError(t, err)

		err = stakingIndexer.Start()
		require.NoError(t, err)
		defer func() {
			err := stakingIndexer.Stop()
			require.NoError(t, err)
		}()

		// 1. build staking tx and insert them into blocks
		// and send block to the confirmed block channel
		totalNumTxs := 0
		numBlocks := r.Intn(100) + 1
		stakingDataList := make([]*datagen.TestStakingData, 0)
		startingHeight := r.Int31n(1000) + 1
		// test staking params
		numCovenantKeys := r.Intn(7) + 3
		quorum := uint32(numCovenantKeys - 2)
		testParams := datagen.GenerateTestStakingParams(t, r, numCovenantKeys, quorum)
		go func() {
			for i := 0; i < numBlocks; i++ {
				numTxs := r.Intn(10) + 1
				totalNumTxs += numTxs
				txs := make([]*btcutil.Tx, 0)
				for j := 0; j < numTxs; j++ {
					stakingData := datagen.GenerateTestStakingData(t, r, 1)
					stakingDataList = append(stakingDataList, stakingData)
					_, tx := datagen.GenerateTxFromTestData(t, testParams, stakingData)
					txs = append(txs, tx)

				}
				b := &vtypes.IndexedBlock{
					Height: startingHeight + int32(i),
					Txs:    txs,
				}
				confirmedBlockChan <- b
			}
		}()

		// 2. read the staking event channel expect them to be the
		// same as the data before being inserted into the block
		stakingEventChan := stakingIndexer.StakingEventChan()
		for i := 0; i < totalNumTxs; i++ {
			ev := <-stakingEventChan
			expectedStakerKeyHex := hex.EncodeToString(schnorr.SerializePubKey(stakingDataList[i].StakerKey))
			expectedFpKeyHex := hex.EncodeToString(schnorr.SerializePubKey(stakingDataList[i].FinalityProviderKeys[0]))
			require.Equal(t, expectedStakerKeyHex, ev.StakerPkHex)
			require.Equal(t, stakingDataList[0].StakingTime, ev.StakingLength)
			require.Equal(t, uint64(stakingDataList[0].StakingAmount), ev.StakingValue)
			require.Equal(t, expectedFpKeyHex, ev.FinalityProviderPkHex)
		}
	})
}

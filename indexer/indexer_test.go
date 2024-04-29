package indexer_test

import (
	"math/rand"
	"path/filepath"
	"sync"
	"testing"
	"time"

	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/indexer"
	"github.com/babylonchain/staking-indexer/testutils"
	"github.com/babylonchain/staking-indexer/testutils/datagen"
	"github.com/babylonchain/staking-indexer/testutils/mocks"
	"github.com/babylonchain/staking-indexer/types"
)

// FuzzIndexer tests the property that the indexer can correctly
// parse staking tx from confirmed blocks
func FuzzIndexer(f *testing.F) {
	// use small seed because db open/close is slow
	bbndatagen.AddRandomSeedsToFuzzer(f, 3)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		homePath := filepath.Join(t.TempDir(), "indexer")
		cfg := config.DefaultConfigWithHome(homePath)

		confirmedBlockChan := make(chan *types.IndexedBlock)
		sysParams := datagen.GenerateGlobalParams(r, t)

		db, err := cfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)
		mockBtcScanner := NewMockedBtcScanner(t, confirmedBlockChan)
		stakingIndexer, err := indexer.NewStakingIndexer(cfg, zap.NewNop(), NewMockedConsumer(t), db, sysParams, mockBtcScanner)
		require.NoError(t, err)

		err = stakingIndexer.Start(1)
		require.NoError(t, err)
		defer func() {
			err := stakingIndexer.Stop()
			require.NoError(t, err)
			err = db.Close()
			require.NoError(t, err)
		}()

		// 1. build staking tx and insert them into blocks
		// and send block to the confirmed block channel
		numBlocks := r.Intn(10) + 1
		startingHeight := r.Int31n(1000) + 1

		stakingDataList := make([]*datagen.TestStakingData, 0)
		stakingTxList := make([]*btcutil.Tx, 0)
		unbondingTxList := make([]*btcutil.Tx, 0)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < numBlocks; i++ {
				numTxs := r.Intn(10) + 1
				blockTxs := make([]*btcutil.Tx, 0)
				for j := 0; j < numTxs; j++ {
					stakingData := datagen.GenerateTestStakingData(t, r)
					stakingDataList = append(stakingDataList, stakingData)
					_, stakingTx := datagen.GenerateStakingTxFromTestData(t, r, sysParams, stakingData)
					unbondingTx := datagen.GenerateUnbondingTxFromStaking(t, sysParams, stakingData, stakingTx.Hash(), 0)
					blockTxs = append(blockTxs, stakingTx)
					blockTxs = append(blockTxs, unbondingTx)
					stakingTxList = append(stakingTxList, stakingTx)
					unbondingTxList = append(unbondingTxList, unbondingTx)
				}
				b := &types.IndexedBlock{
					Height: startingHeight + int32(i),
					Txs:    blockTxs,
					Header: &wire.BlockHeader{Timestamp: time.Now()},
				}
				confirmedBlockChan <- b
			}
		}()
		wg.Wait()

		// wait for db writes finished
		time.Sleep(2 * time.Second)

		// 2. read local store and expect them to be the
		// same as the data before being stored
		for i := 0; i < len(stakingTxList); i++ {
			tx := stakingTxList[i].MsgTx()
			txHash := tx.TxHash()
			data := stakingDataList[i]
			storedTx, err := stakingIndexer.GetStakingTxByHash(&txHash)
			require.NoError(t, err)
			require.Equal(t, tx.TxHash(), storedTx.Tx.TxHash())
			require.True(t, testutils.PubKeysEqual(data.StakerKey, storedTx.StakerPk))
			require.Equal(t, uint32(data.StakingTime), storedTx.StakingTime)
			require.True(t, testutils.PubKeysEqual(data.FinalityProviderKey, storedTx.FinalityProviderPk))
		}

		for i := 0; i < len(unbondingTxList); i++ {
			tx := unbondingTxList[i].MsgTx()
			txHash := tx.TxHash()
			expectedStakingTx := stakingTxList[i].MsgTx()
			storedUnbondingTx, err := stakingIndexer.GetUnbondingTxByHash(&txHash)
			require.NoError(t, err)
			require.Equal(t, tx.TxHash(), storedUnbondingTx.Tx.TxHash())
			require.Equal(t, expectedStakingTx.TxHash().String(), storedUnbondingTx.StakingTxHash.String())

			expectedStakingData := stakingDataList[i]
			storedStakingTx, err := stakingIndexer.GetStakingTxByHash(storedUnbondingTx.StakingTxHash)
			require.NoError(t, err)
			require.Equal(t, expectedStakingTx.TxHash(), storedStakingTx.Tx.TxHash())
			require.True(t, testutils.PubKeysEqual(expectedStakingData.StakerKey, storedStakingTx.StakerPk))
			require.Equal(t, uint32(expectedStakingData.StakingTime), storedStakingTx.StakingTime)
			require.True(t, testutils.PubKeysEqual(expectedStakingData.FinalityProviderKey, storedStakingTx.FinalityProviderPk))
		}
	})
}

func FuzzGetStartHeight(f *testing.F) {
	// use small seed because db open/close is slow
	bbndatagen.AddRandomSeedsToFuzzer(f, 3)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		homePath := filepath.Join(t.TempDir(), "indexer")
		cfg := config.DefaultConfigWithHome(homePath)

		confirmedBlockChan := make(chan *types.IndexedBlock)
		sysParams := datagen.GenerateGlobalParams(r, t)

		db, err := cfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)
		mockBtcScanner := NewMockedBtcScanner(t, confirmedBlockChan)
		stakingIndexer, err := indexer.NewStakingIndexer(cfg, zap.NewNop(), NewMockedConsumer(t), db, sysParams, mockBtcScanner)
		require.NoError(t, err)

		// 1. no blocks have been processed, the start height should be equal to the base height
		initialHeight := stakingIndexer.GetStartHeight()
		require.Equal(t, cfg.BTCScannerConfig.BaseHeight, initialHeight)
		err = stakingIndexer.ValidateStartHeight(initialHeight)
		require.NoError(t, err)

		err = stakingIndexer.Start(1)
		require.NoError(t, err)
		defer func() {
			err := stakingIndexer.Stop()
			require.NoError(t, err)
			err = db.Close()
			require.NoError(t, err)
		}()

		// 2. generate some blocks and let the last processed height grow
		numBlocks := r.Intn(10) + 1
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < numBlocks; i++ {
				b := &types.IndexedBlock{
					Height: int32(initialHeight) + int32(i),
				}
				confirmedBlockChan <- b
			}
		}()
		wg.Wait()

		// wait for db writes finished
		time.Sleep(2 * time.Second)

		// get start height from the indexer again, it should be equal to
		// the last processed height (initialHeight + numBlocks - 1) + 1
		startHeight := stakingIndexer.GetStartHeight()
		require.Equal(t, initialHeight+uint64(numBlocks), startHeight)
		err = stakingIndexer.ValidateStartHeight(startHeight)
		require.NoError(t, err)

		// 3. test the case where the start height is between [base height, last processed height + 1]
		gap := int(startHeight - initialHeight)
		validStartHeight := initialHeight + uint64(r.Intn(gap)) + 1
		err = stakingIndexer.ValidateStartHeight(validStartHeight)
		require.NoError(t, err)

		// 4. test the case where the start height is less than the base height
		smallHeight := uint64(r.Intn(int(initialHeight)))
		err = stakingIndexer.ValidateStartHeight(smallHeight)
		require.Error(t, err)

		// 5. test the case where the start height is more than the last processed height + 1
		bigHeight := uint64(r.Intn(1000)) + 1 + startHeight
		err = stakingIndexer.ValidateStartHeight(bigHeight)
		require.Error(t, err)
	})
}

func NewMockedConsumer(t *testing.T) *mocks.MockEventConsumer {
	ctl := gomock.NewController(t)
	mockedConsumer := mocks.NewMockEventConsumer(ctl)
	mockedConsumer.EXPECT().PushStakingEvent(gomock.Any()).Return(nil).AnyTimes()
	mockedConsumer.EXPECT().PushUnbondingEvent(gomock.Any()).Return(nil).AnyTimes()
	mockedConsumer.EXPECT().PushWithdrawEvent(gomock.Any()).Return(nil).AnyTimes()
	mockedConsumer.EXPECT().Start().Return(nil).AnyTimes()
	mockedConsumer.EXPECT().Stop().Return(nil).AnyTimes()

	return mockedConsumer
}

func NewMockedBtcScanner(t *testing.T, confirmedBlocksChan chan *types.IndexedBlock) *mocks.MockBtcScanner {
	ctl := gomock.NewController(t)
	mockBtcScanner := mocks.NewMockBtcScanner(ctl)
	mockBtcScanner.EXPECT().Start(gomock.Any()).Return(nil).AnyTimes()
	mockBtcScanner.EXPECT().ConfirmedBlocksChan().Return(confirmedBlocksChan).AnyTimes()
	mockBtcScanner.EXPECT().Stop().Return(nil).AnyTimes()

	return mockBtcScanner
}

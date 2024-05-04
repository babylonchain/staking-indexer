package indexer_test

import (
	"math/rand"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/babylonchain/babylon/btcstaking"
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

type StakingTxData struct {
	StakingTx   *btcutil.Tx
	StakingData *datagen.TestStakingData
	Unbonded    bool
}

// FuzzIndexer tests the property that the indexer can correctly
// parse staking tx from confirmed blocks
func FuzzIndexer(f *testing.F) {
	// use small seed because db open/close is slow
	bbndatagen.AddRandomSeedsToFuzzer(f, 5)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		homePath := filepath.Join(t.TempDir(), "indexer")
		cfg := config.DefaultConfigWithHome(homePath)

		confirmedBlockChan := make(chan *types.IndexedBlock)
		sysParamsVersions := datagen.GenerateGlobalParamsVersions(r, t)

		db, err := cfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)
		mockBtcScanner := NewMockedBtcScanner(t, confirmedBlockChan)
		stakingIndexer, err := indexer.NewStakingIndexer(cfg, zap.NewNop(), NewMockedConsumer(t), db, sysParamsVersions, mockBtcScanner)
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
		// Starting height should be at least later than the activation height of the first parameters
		startingHeight := r.Int31n(1000) + 1 + sysParamsVersions.ParamsVersions[0].ActivationHeight

		stakingDataList := make([]*datagen.TestStakingData, 0)
		stakingTxList := make([]*btcutil.Tx, 0)
		unbondingTxList := make([]*btcutil.Tx, 0)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < numBlocks; i++ {
				blockHeight := startingHeight + int32(i)
				params, err := sysParamsVersions.GetParamsForBTCHeight(blockHeight)
				require.NoError(t, err)
				numTxs := r.Intn(10) + 1
				blockTxs := make([]*btcutil.Tx, 0)
				for j := 0; j < numTxs; j++ {
					stakingData := datagen.GenerateTestStakingData(t, r, params)
					stakingDataList = append(stakingDataList, stakingData)
					_, stakingTx := datagen.GenerateStakingTxFromTestData(t, r, params, stakingData)
					unbondingTx := datagen.GenerateUnbondingTxFromStaking(t, params, stakingData, stakingTx.Hash(), 0)
					blockTxs = append(blockTxs, stakingTx)
					blockTxs = append(blockTxs, unbondingTx)
					stakingTxList = append(stakingTxList, stakingTx)
					unbondingTxList = append(unbondingTxList, unbondingTx)
				}
				b := &types.IndexedBlock{
					Height: blockHeight,
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

// FuzzVerifyUnbondingTx tests IsValidUnbondingTx in three scenarios:
// 1. it returns (true, nil) if the given tx is valid unbonding tx
// 2. it returns (false, nil) if the given tx is not unbonding tx
// 3. it returns (false, ErrInvalidUnbondingTx) if the given tx is not
// a valid unbonding tx
func FuzzVerifyUnbondingTx(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 50)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		homePath := filepath.Join(t.TempDir(), "indexer")
		cfg := config.DefaultConfigWithHome(homePath)

		confirmedBlockChan := make(chan *types.IndexedBlock)
		sysParamsVersions := datagen.GenerateGlobalParamsVersions(r, t)

		db, err := cfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)
		mockBtcScanner := NewMockedBtcScanner(t, confirmedBlockChan)
		stakingIndexer, err := indexer.NewStakingIndexer(cfg, zap.NewNop(), NewMockedConsumer(t), db, sysParamsVersions, mockBtcScanner)
		require.NoError(t, err)
		defer func() {
			err = db.Close()
			require.NoError(t, err)
		}()

		// Select the first params versions to play with
		params := sysParamsVersions.ParamsVersions[0]
		// 1. generate and add a valid staking tx to the indexer
		stakingData := datagen.GenerateTestStakingData(t, r, params)
		_, stakingTx := datagen.GenerateStakingTxFromTestData(t, r, params, stakingData)
		// For a valid tx, its btc height is always larger than the activation height
		mockedHeight := uint64(params.ActivationHeight) + 1
		err = stakingIndexer.ProcessStakingTx(
			stakingTx.MsgTx(),
			getParsedStakingData(stakingData, stakingTx.MsgTx(), params),
			mockedHeight, time.Now(), params)
		require.NoError(t, err)
		storedStakingTx, err := stakingIndexer.GetStakingTxByHash(stakingTx.Hash())
		require.NoError(t, err)

		// 2. test IsValidUnbondingTx with valid unbonding tx, expect (true, nil)
		unbondingTx := datagen.GenerateUnbondingTxFromStaking(t, params, stakingData, stakingTx.Hash(), 0)
		isValid, err := stakingIndexer.IsValidUnbondingTx(unbondingTx.MsgTx(), storedStakingTx, params)
		require.NoError(t, err)
		require.True(t, isValid)

		// 3. test IsValidUnbondingTx with no unbonding tx (different staking output index), expect (false, nil)
		unbondingTx = datagen.GenerateUnbondingTxFromStaking(t, params, stakingData, stakingTx.Hash(), 1)
		isValid, err = stakingIndexer.IsValidUnbondingTx(unbondingTx.MsgTx(), storedStakingTx, params)
		require.NoError(t, err)
		require.False(t, isValid)

		// 4. test IsValidUnbondingTx with invalid unbonding tx (random unbonding fee in params), expect (false, ErrInvalidUnbondingTx)
		newParams := *params
		newParams.UnbondingFee = btcutil.Amount(bbndatagen.RandomIntOtherThan(r, int(params.UnbondingFee), 10000000))
		unbondingTx = datagen.GenerateUnbondingTxFromStaking(t, &newParams, stakingData, stakingTx.Hash(), 0)
		// pass the old params
		isValid, err = stakingIndexer.IsValidUnbondingTx(unbondingTx.MsgTx(), storedStakingTx, params)
		require.ErrorIs(t, err, indexer.ErrInvalidUnbondingTx)
		require.False(t, isValid)
	})
}

func FuzzTestOverflow(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		homePath := filepath.Join(t.TempDir(), "indexer")
		cfg := config.DefaultConfigWithHome(homePath)

		confirmedBlockChan := make(chan *types.IndexedBlock)
		sysParamsVersions := datagen.GenerateGlobalParamsVersions(r, t)

		db, err := cfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)
		mockBtcScanner := NewMockedBtcScanner(t, confirmedBlockChan)
		stakingIndexer, err := indexer.NewStakingIndexer(cfg, zap.NewNop(), NewMockedConsumer(t), db, sysParamsVersions, mockBtcScanner)
		require.NoError(t, err)
		defer func() {
			err = db.Close()
			require.NoError(t, err)
		}()

		// Select the first params versions to play with
		params := sysParamsVersions.ParamsVersions[0]
		// Accumulate the test data
		var stakingTxData []*StakingTxData
		// Keep sending staking tx until the staking cap is reached for the very last tx
		for {
			stakingData := datagen.GenerateTestStakingData(t, r, params)
			_, stakingTx := datagen.GenerateStakingTxFromTestData(t, r, params, stakingData)
			// For a valid tx, its btc height is always larger than the activation height
			mockedHeight := uint64(params.ActivationHeight) + 1

			tvl, err := stakingIndexer.GetConfirmedTvl()
			require.NoError(t, err)

			err = stakingIndexer.ProcessStakingTx(
				stakingTx.MsgTx(),
				getParsedStakingData(stakingData, stakingTx.MsgTx(), params),
				mockedHeight, time.Now(), params)
			require.NoError(t, err)
			storedStakingTx, err := stakingIndexer.GetStakingTxByHash(stakingTx.Hash())
			require.NoError(t, err)

			stakingTxData = append(stakingTxData, &StakingTxData{
				StakingTx:   stakingTx,
				StakingData: stakingData,
				Unbonded:    false,
			})
			// ~10% chance to trigger unbonding tx on existing staking tx to reduce the tvl
			if r.Intn(10) == 0 {
				sendUnbondingTx(t, stakingIndexer, params, stakingTxData, r)
			}

			// Let's break if the staking tx is overflow
			if storedStakingTx.IsOverflow {
				require.True(t, tvl+int64(stakingData.StakingAmount) > int64(params.StakingCap))
				break
			}
			require.True(t, tvl+int64(stakingData.StakingAmount) <= int64(params.StakingCap))
		}

		// Now, let's test the overflow with the second params
		secondParam := sysParamsVersions.ParamsVersions[1]

		// Let's send more staking txs until the staking cap is exceeded again
		for {
			stakingData := datagen.GenerateTestStakingData(t, r, secondParam)
			_, stakingTx := datagen.GenerateStakingTxFromTestData(t, r, secondParam, stakingData)
			// For a valid tx, its btc height is always larger than the activation height
			mockedHeight := uint64(secondParam.ActivationHeight) + 1
			tvl, err := stakingIndexer.GetConfirmedTvl()
			require.NoError(t, err)
			err = stakingIndexer.ProcessStakingTx(
				stakingTx.MsgTx(),
				getParsedStakingData(stakingData, stakingTx.MsgTx(), secondParam),
				mockedHeight, time.Now(), secondParam)
			require.NoError(t, err)
			storedStakingTx, err := stakingIndexer.GetStakingTxByHash(stakingTx.Hash())
			require.NoError(t, err)

			stakingTxData = append(stakingTxData, &StakingTxData{
				StakingTx:   stakingTx,
				StakingData: stakingData,
			})
			// ~20% chance to trigger unbonding tx on existing staking tx to reduce the tvl
			if r.Intn(5) == 0 {
				sendUnbondingTx(t, stakingIndexer, secondParam, stakingTxData, r)
			}

			// Let's break if the staking tx is overflow
			if storedStakingTx.IsOverflow {
				require.True(t, tvl+int64(stakingData.StakingAmount) > int64(secondParam.StakingCap))
				break
			}
			require.True(t, tvl+int64(stakingData.StakingAmount) <= int64(secondParam.StakingCap))
		}
	})
}

func getParsedStakingData(data *datagen.TestStakingData, tx *wire.MsgTx, params *types.Params) *btcstaking.ParsedV0StakingTx {
	return &btcstaking.ParsedV0StakingTx{
		StakingOutput:     tx.TxOut[0],
		StakingOutputIdx:  0,
		OpReturnOutput:    tx.TxOut[1],
		OpReturnOutputIdx: 1,
		OpReturnData: &btcstaking.V0OpReturnData{
			MagicBytes:                params.Tag,
			Version:                   0,
			StakerPublicKey:           &btcstaking.XonlyPubKey{PubKey: data.StakerKey},
			FinalityProviderPublicKey: &btcstaking.XonlyPubKey{PubKey: data.FinalityProviderKey},
			StakingTime:               data.StakingTime,
		},
	}
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

// This helper method will randomly select a staking tx from stakingTxData and unbond it
func sendUnbondingTx(
	t *testing.T, stakingIndexer *indexer.StakingIndexer,
	params *types.Params, stakingTxData []*StakingTxData, r *rand.Rand,
) {
	// select a random staking tx from stakingTxData and unbond it if it is not already unbonded
	data := stakingTxData[r.Intn(len(stakingTxData))]
	if data.Unbonded {
		return
	}
	// unbond the staking tx
	unbondingTx := datagen.GenerateUnbondingTxFromStaking(t, params, data.StakingData, data.StakingTx.Hash(), 0)
	mockedHeight := uint64(params.ActivationHeight) + 1
	err := stakingIndexer.ProcessUnbondingTx(
		unbondingTx.MsgTx(),
		data.StakingTx.Hash(),
		mockedHeight, time.Now(), params)
	require.NoError(t, err)
	data.Unbonded = true
}

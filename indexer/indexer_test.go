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
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/btcscanner"
	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/indexer"
	"github.com/babylonchain/staking-indexer/indexerstore"
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

type StakingEvent struct {
	StakingTx     *btcutil.Tx
	StakingTxData *datagen.TestStakingData
	Height        int32
	Unbonded      bool
	IsOverflow    bool
}

type UnbondingEvent struct {
	StakingTxHash *chainhash.Hash
	UnbondingTx   *btcutil.Tx
	Height        int32
}

type TestScenario struct {
	VersionedParams *types.ParamsVersions
	StakingEvents   []*StakingEvent
	UnbondingEvents []*UnbondingEvent
	Blocks          []*types.IndexedBlock
	TvlToHeight     map[int32]btcutil.Amount
	Tvl             btcutil.Amount
}

// NewTestScenario creates a scenario where staking txs and unbonding txs are
// randomly distributed across blocks. A tx has a given chance (stakingChance/100)
// to be a staking tx while the rest to be an unbonding tx, n is the number of
// staking txs and unbonding txs
func NewTestScenario(r *rand.Rand, t *testing.T, versionedParams *types.ParamsVersions, stakingChance int, numEvents int, checkOverflow bool) *TestScenario {
	startHeight := r.Int31n(1000) + 1 + int32(versionedParams.ParamsVersions[0].ActivationHeight)
	lastEventHeight := startHeight
	stakingEvents := make([]*StakingEvent, 0)
	unbondingEvents := make([]*UnbondingEvent, 0)
	tvl := btcutil.Amount(0)
	txsPerHeight := make(map[int32][]*btcutil.Tx)
	tvlToHeight := make(map[int32]btcutil.Amount)

	// create numEvents events
	for i := 0; i < numEvents; i++ {
		// randomly select a height at which the event should be happening
		height := lastEventHeight + r.Int31n(3)
		p, err := versionedParams.GetParamsForBTCHeight(height)
		require.NoError(t, err)
		txs, ok := txsPerHeight[height]
		if !ok {
			// new height
			txs = make([]*btcutil.Tx, 0)
		}

		// stakingChance/100 chance to be a staking event, or there are
		// no active staking events created, otherwise, to be an unbonding event
		if r.Intn(100) < stakingChance || !hasActiveStakingEvent(stakingEvents) {
			stakingEvent := buildStakingEvent(r, t, height, p)
			if checkOverflow && tvl >= p.StakingCap {
				stakingEvent.IsOverflow = true
			} else {
				tvl += stakingEvent.StakingTxData.StakingAmount
			}
			stakingEvents = append(stakingEvents, stakingEvent)
			txs = append(txs, stakingEvent.StakingTx)
		} else {
			prevStakingEvent := findActiveStakingEvent(stakingEvents, r)
			require.NotNil(t, prevStakingEvent)
			require.False(t, prevStakingEvent.Unbonded)
			stakingParams, err := versionedParams.GetParamsForBTCHeight(prevStakingEvent.Height)
			require.NoError(t, err)
			unbondingEvent := buildUnbondingEvent(prevStakingEvent, height, stakingParams, t)
			unbondingEvents = append(unbondingEvents, unbondingEvent)
			prevStakingEvent.Unbonded = true
			if !prevStakingEvent.IsOverflow {
				tvl -= prevStakingEvent.StakingTxData.StakingAmount
			}
			require.True(t, tvl >= 0)
			txs = append(txs, unbondingEvent.UnbondingTx)
		}

		txsPerHeight[height] = txs
		tvlToHeight[height] = tvl
		lastEventHeight = height
	}

	blocks := make([]*types.IndexedBlock, 0)
	for h := startHeight; h <= lastEventHeight; h++ {
		block := &types.IndexedBlock{
			Height: h,
			Header: &wire.BlockHeader{Timestamp: time.Now()},
			Txs:    txsPerHeight[h],
		}
		blocks = append(blocks, block)
		_, ok := tvlToHeight[h]
		if !ok {
			tvlToHeight[h] = tvlToHeight[h-1]
		}
	}

	return &TestScenario{
		VersionedParams: versionedParams,
		StakingEvents:   stakingEvents,
		UnbondingEvents: unbondingEvents,
		Blocks:          blocks,
		TvlToHeight:     tvlToHeight,
		Tvl:             tvl,
	}
}

func buildUnbondingEvent(stakingEvent *StakingEvent, height int32, p *types.GlobalParams, t *testing.T) *UnbondingEvent {
	stakingTxHash := stakingEvent.StakingTx.Hash()
	unbondingTx := datagen.GenerateUnbondingTxFromStaking(t, p, stakingEvent.StakingTxData, stakingTxHash, 0)

	return &UnbondingEvent{
		StakingTxHash: stakingTxHash,
		UnbondingTx:   unbondingTx,
		Height:        height,
	}
}

func findActiveStakingEvent(stakingEvents []*StakingEvent, r *rand.Rand) *StakingEvent {
	var activeStakingEvents []*StakingEvent
	for _, se := range stakingEvents {
		if !se.Unbonded {
			activeStakingEvents = append(activeStakingEvents, se)
		}
	}
	if len(activeStakingEvents) == 0 {
		return nil
	}
	return activeStakingEvents[r.Intn(len(activeStakingEvents))]
}

func hasActiveStakingEvent(stakingEvents []*StakingEvent) bool {
	for _, se := range stakingEvents {
		if !se.Unbonded {
			return true
		}
	}

	return false
}

func buildStakingEvent(r *rand.Rand, t *testing.T, height int32, p *types.GlobalParams) *StakingEvent {
	stakingData := datagen.GenerateTestStakingData(t, r, p)
	_, stakingTx := datagen.GenerateStakingTxFromTestData(t, r, p, stakingData)

	return &StakingEvent{
		StakingTx:     stakingTx,
		StakingTxData: stakingData,
		Height:        height,
	}
}

// FuzzBlockHandler tests the property that the indexer can correctly
// parse staking tx from confirmed blocks
func FuzzBlockHandler(f *testing.F) {
	// Note: before committing, it should be tested with large seed
	// to avoid flaky
	// small seed for ci because db open/close is slow
	bbndatagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		homePath := filepath.Join(t.TempDir(), "indexer")
		cfg := config.DefaultConfigWithHome(homePath)

		n := r.Intn(100) + 1
		sysParamsVersions := datagen.GenerateGlobalParamsVersions(r, t)
		testScenario := NewTestScenario(r, t, sysParamsVersions, 80, n, true)

		db, err := cfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)
		chainUpdateInfoChan := make(chan *btcscanner.ChainUpdateInfo)
		mockBtcScanner := NewMockedBtcScanner(t, chainUpdateInfoChan)
		stakingIndexer, err := indexer.NewStakingIndexer(cfg, zap.NewNop(), NewMockedConsumer(t), db, sysParamsVersions, mockBtcScanner)
		require.NoError(t, err)

		defer func() {
			err = db.Close()
			require.NoError(t, err)
		}()

		for _, b := range testScenario.Blocks {
			err := stakingIndexer.HandleConfirmedBlock(b)
			require.NoError(t, err)
			tvl, err := stakingIndexer.GetConfirmedTvl()
			require.NoError(t, err)
			require.Equal(t, uint64(testScenario.TvlToHeight[b.Height]), tvl)
		}
		tvl, err := stakingIndexer.GetConfirmedTvl()
		require.NoError(t, err)
		require.Equal(t, uint64(testScenario.Tvl), tvl)

		for _, stakingEv := range testScenario.StakingEvents {
			storedTx, err := stakingIndexer.GetStakingTxByHash(stakingEv.StakingTx.Hash())
			require.NoError(t, err)
			require.NotNil(t, storedTx)
			require.Equal(t, stakingEv.StakingTx.Hash().String(), storedTx.Tx.TxHash().String())
			require.True(t, testutils.PubKeysEqual(stakingEv.StakingTxData.StakerKey, storedTx.StakerPk))
			require.Equal(t, uint32(stakingEv.StakingTxData.StakingTime), storedTx.StakingTime)
			require.True(t, testutils.PubKeysEqual(stakingEv.StakingTxData.FinalityProviderKey, storedTx.FinalityProviderPk))
			require.Equal(t, stakingEv.IsOverflow, storedTx.IsOverflow)
		}

		for _, unbondingEv := range testScenario.UnbondingEvents {
			storedTx, err := stakingIndexer.GetUnbondingTxByHash(unbondingEv.UnbondingTx.Hash())
			require.NoError(t, err)
			require.NotNil(t, storedTx)
			require.Equal(t, unbondingEv.StakingTxHash, storedTx.StakingTxHash)
			require.Equal(t, unbondingEv.UnbondingTx.Hash().String(), storedTx.Tx.TxHash().String())
		}

		// replay the blocks and the result should be the same
		for _, b := range testScenario.Blocks {
			err := stakingIndexer.HandleConfirmedBlock(b)
			require.NoError(t, err)
		}
		tvl, err = stakingIndexer.GetConfirmedTvl()
		require.NoError(t, err)
		require.Equal(t, uint64(testScenario.Tvl), tvl)

		for _, stakingEv := range testScenario.StakingEvents {
			storedTx, err := stakingIndexer.GetStakingTxByHash(stakingEv.StakingTx.Hash())
			require.NoError(t, err)
			require.NotNil(t, storedTx)
			require.Equal(t, stakingEv.StakingTx.Hash().String(), storedTx.Tx.TxHash().String())
			require.True(t, testutils.PubKeysEqual(stakingEv.StakingTxData.StakerKey, storedTx.StakerPk))
			require.Equal(t, uint32(stakingEv.StakingTxData.StakingTime), storedTx.StakingTime)
			require.True(t, testutils.PubKeysEqual(stakingEv.StakingTxData.FinalityProviderKey, storedTx.FinalityProviderPk))
			require.Equal(t, stakingEv.IsOverflow, storedTx.IsOverflow)
		}

		for _, unbondingEv := range testScenario.UnbondingEvents {
			storedTx, err := stakingIndexer.GetUnbondingTxByHash(unbondingEv.UnbondingTx.Hash())
			require.NoError(t, err)
			require.NotNil(t, storedTx)
			require.Equal(t, unbondingEv.StakingTxHash, storedTx.StakingTxHash)
			require.Equal(t, unbondingEv.UnbondingTx.Hash().String(), storedTx.Tx.TxHash().String())
		}

		// calculate unconfirmed tvl
		testUnconfirmedScenario := NewTestScenario(r, t, sysParamsVersions, 80, n, false)
		unconfirmedTvl, err := stakingIndexer.CalculateTvlInUnconfirmedBlocks(testUnconfirmedScenario.Blocks)
		require.NoError(t, err)
		require.Equal(t, testUnconfirmedScenario.Tvl, unconfirmedTvl)
	})
}

func FuzzGetStartHeight(f *testing.F) {
	// use small seed because db open/close is slow
	bbndatagen.AddRandomSeedsToFuzzer(f, 6)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		homePath := filepath.Join(t.TempDir(), "indexer")
		cfg := config.DefaultConfigWithHome(homePath)

		sysParams := datagen.GenerateGlobalParamsVersions(r, t)

		db, err := cfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)
		chainUpdateInfoChan := make(chan *btcscanner.ChainUpdateInfo)
		mockBtcScanner := NewMockedBtcScanner(t, chainUpdateInfoChan)
		mockBtcScanner.EXPECT().IsSynced().Return(false).AnyTimes()
		stakingIndexer, err := indexer.NewStakingIndexer(cfg, zap.NewNop(), NewMockedConsumer(t), db, sysParams, mockBtcScanner)
		require.NoError(t, err)

		// 1. no blocks have been processed, the start height should be equal to the base height
		initialHeight := stakingIndexer.GetStartHeight()
		require.Equal(t, sysParams.ParamsVersions[0].ActivationHeight, initialHeight)
		err = stakingIndexer.ValidateStartHeight(initialHeight)
		require.NoError(t, err)

		err = stakingIndexer.ValidateStartHeight(bbndatagen.RandomIntOtherThan(r, int(initialHeight), 100))
		require.Error(t, err)

		err = stakingIndexer.Start(initialHeight)
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
			confirmedBlocks := make([]*types.IndexedBlock, 0)
			for i := 0; i < numBlocks; i++ {
				b := &types.IndexedBlock{
					Height: int32(initialHeight) + int32(i),
				}
				confirmedBlocks = append(confirmedBlocks, b)
			}
			chainUpdateInfoChan <- &btcscanner.ChainUpdateInfo{
				ConfirmedBlocks: confirmedBlocks,
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

		sysParamsVersions := datagen.GenerateGlobalParamsVersions(r, t)

		db, err := cfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)
		chainUpdateInfoChan := make(chan *btcscanner.ChainUpdateInfo)
		mockBtcScanner := NewMockedBtcScanner(t, chainUpdateInfoChan)
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
		require.NotNil(t, storedStakingTx)

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

		// 5. test IsValidUnbondingTx with invalid unbonding tx (random unbonding time in params), expect (false, ErrInvalidUnbondingTx)
		newParams2 := *params
		newParams2.UnbondingTime = uint16(bbndatagen.RandomIntOtherThan(r, int(params.UnbondingTime), 1000))
		unbondingTx = datagen.GenerateUnbondingTxFromStaking(t, &newParams2, stakingData, stakingTx.Hash(), 0)
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

		sysParamsVersions := datagen.GenerateGlobalParamsVersions(r, t)

		db, err := cfg.DatabaseConfig.GetDbBackend()
		require.NoError(t, err)
		chainUpdateInfoChan := make(chan *btcscanner.ChainUpdateInfo)
		mockBtcScanner := NewMockedBtcScanner(t, chainUpdateInfoChan)
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
			stakingData, tvl, storedStakingTx, stakingTx := sendStakingTx(
				t, r, stakingIndexer, params, stakingTxData,
				uint64(params.ActivationHeight)+1,
			)
			stakingTxData = append(stakingTxData, &StakingTxData{
				StakingTx:   stakingTx,
				StakingData: stakingData,
			})
			// ~10% chance to trigger unbonding tx on existing staking tx to reduce the tvl
			if r.Intn(10) == 0 {
				sendUnbondingTx(t, stakingIndexer, params, stakingTxData, r)
			}

			// Let's break if the staking tx is overflow
			if storedStakingTx.IsOverflow {
				require.True(t, tvl > uint64(params.StakingCap))
				break
			}
			require.True(t, tvl <= uint64(params.StakingCap))
		}

		// Unbond some of the tx so that new staking tx can be processed
		for {
			sendUnbondingTx(t, stakingIndexer, params, stakingTxData, r)
			// Let's break if the tvl is below max staking value
			tvl, err := stakingIndexer.GetConfirmedTvl()
			require.NoError(t, err)
			if tvl < uint64(params.StakingCap) {
				break
			}
		}

		// Send staking tx again so that it will be accepted until overflow again
		for {
			stakingData, tvl, storedStakingTx, stakingTx := sendStakingTx(
				t, r, stakingIndexer, params, stakingTxData,
				uint64(params.ActivationHeight)+1,
			)
			stakingTxData = append(stakingTxData, &StakingTxData{
				StakingTx:   stakingTx,
				StakingData: stakingData,
			})
			if storedStakingTx.IsOverflow {
				require.True(t, tvl > uint64(params.StakingCap))
				break
			}
			require.True(t, tvl <= uint64(params.StakingCap))
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
			require.NotNil(t, storedStakingTx)

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
				require.True(t, tvl > uint64(secondParam.StakingCap))
				break
			}
			require.True(t, tvl <= uint64(secondParam.StakingCap))
		}
	})
}

func getParsedStakingData(data *datagen.TestStakingData, tx *wire.MsgTx, params *types.GlobalParams) *btcstaking.ParsedV0StakingTx {
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

func NewMockedBtcScanner(t *testing.T, chainUpdateInfoChan chan *btcscanner.ChainUpdateInfo) *mocks.MockBtcScanner {
	ctl := gomock.NewController(t)
	mockBtcScanner := mocks.NewMockBtcScanner(ctl)
	mockBtcScanner.EXPECT().Start(gomock.Any()).Return(nil).AnyTimes()
	mockBtcScanner.EXPECT().ChainUpdateInfoChan().Return(chainUpdateInfoChan).AnyTimes()
	mockBtcScanner.EXPECT().Stop().Return(nil).AnyTimes()

	return mockBtcScanner
}

// This helper method will randomly select a staking tx from stakingTxData and unbond it
func sendUnbondingTx(
	t *testing.T, stakingIndexer *indexer.StakingIndexer,
	params *types.GlobalParams, stakingTxData []*StakingTxData, r *rand.Rand,
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

func sendStakingTx(
	t *testing.T, r *rand.Rand, stakingIndexer *indexer.StakingIndexer,
	params *types.GlobalParams, stakingTxData []*StakingTxData, height uint64,
) (*datagen.TestStakingData, uint64, *indexerstore.StoredStakingTransaction, *btcutil.Tx) {
	stakingData := datagen.GenerateTestStakingData(t, r, params)
	_, stakingTx := datagen.GenerateStakingTxFromTestData(t, r, params, stakingData)
	tvl, err := stakingIndexer.GetConfirmedTvl()
	require.NoError(t, err)
	err = stakingIndexer.ProcessStakingTx(
		stakingTx.MsgTx(),
		getParsedStakingData(stakingData, stakingTx.MsgTx(), params),
		height, time.Now(), params)
	require.NoError(t, err)
	storedStakingTx, err := stakingIndexer.GetStakingTxByHash(stakingTx.Hash())
	require.NoError(t, err)
	require.NotNil(t, storedStakingTx)

	return stakingData, tvl, storedStakingTx, stakingTx
}

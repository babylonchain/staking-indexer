package indexer_test

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/babylonchain/babylon/btcstaking"
	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/indexer"
	"github.com/babylonchain/staking-indexer/params"
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

// FuzzVerifyUnbondingTx tests IsValidUnbondingTx in three scenarios:
// 1. it returns (true, nil) if the given tx is valid unbonding tx
// 2. it returns (false, nil) if the given tx is not unbonding tx
// 3. it returns (false, ErrInvalidUnbondingTx) if the given tx is not
// a valid unbonding tx
func FuzzVerifyUnbondingTx(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 10)

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
		defer func() {
			err = db.Close()
			require.NoError(t, err)
		}()

		// 1. generate and add a valid staking tx to the indexer
		stakingData := datagen.GenerateTestStakingData(t, r)
		_, stakingTx := datagen.GenerateStakingTxFromTestData(t, r, sysParams, stakingData)
		err = stakingIndexer.ProcessStakingTx(
			stakingTx.MsgTx(),
			getParsedStakingData(stakingData, stakingTx.MsgTx(), sysParams),
			100, time.Now())
		require.NoError(t, err)
		storedStakingTx, err := stakingIndexer.GetStakingTxByHash(stakingTx.Hash())
		require.NoError(t, err)

		// 2. test IsValidUnbondingTx with valid unbonding tx, expect (true, nil)
		unbondingTx := datagen.GenerateUnbondingTxFromStaking(t, sysParams, stakingData, stakingTx.Hash(), 0)
		isValid, err := stakingIndexer.IsValidUnbondingTx(unbondingTx.MsgTx(), storedStakingTx)
		require.NoError(t, err)
		require.True(t, isValid)

		// 3. test IsValidUnbondingTx with no unbonding tx (different staking output index), expect (false, nil)
		unbondingTx = datagen.GenerateUnbondingTxFromStaking(t, sysParams, stakingData, stakingTx.Hash(), 1)
		isValid, err = stakingIndexer.IsValidUnbondingTx(unbondingTx.MsgTx(), storedStakingTx)
		require.NoError(t, err)
		require.False(t, isValid)

		// 4. test IsValidUnbondingTx with invaid unbonding tx (random unbonding fee in params), expect (false, ErrInvalidUnbondingTx)
		newParams := *sysParams
		newParams.UnbondingFee = btcutil.Amount(bbndatagen.RandomIntOtherThan(r, int(sysParams.UnbondingFee), 10000000))
		unbondingTx = datagen.GenerateUnbondingTxFromStaking(t, &newParams, stakingData, stakingTx.Hash(), 0)
		isValid, err = stakingIndexer.IsValidUnbondingTx(unbondingTx.MsgTx(), storedStakingTx)
		require.ErrorIs(t, err, indexer.ErrInvalidUnbondingTx)
		require.False(t, isValid)
	})
}

func getParams(t *testing.T, homepath string) *types.Params {
	paramsRetriever, err := params.NewLocalParamsRetriever(homepath)
	require.NoError(t, err)
	return paramsRetriever.GetParams()
}

func TestStakingParser(t *testing.T) {
	p := getParams(t, "./test-params.json")
	txHex := "02000000000101d9994153032fee252fe4f50f3954ec7131c74582aa4d5bdf072a0db26cdbc8280000000000ffffffff0330750000000000002251206407e8e16d9310c049a12da7db9563c704e0e3e780862e79ef6948733452a0ca0000000000000000496a4762627434004a897d051130f15eacab4bfcc681032246c77cd56a263538ee35b4d9173b2a9603d5a0bb72d71993e435d6c5a70e2aa4db500a62cfaae33c56050deefee64ec000fae0160400000000002251208b41b0129693efa21c9389a04373d2dc2d522aa641600a66c98e7ac4f65b04e00140abc2ed8240b02d78275a879a92e55642ce214e25dacede4b07b904f78da25fe91bfe3c6f50b0f4ad93579ac52109fcae1520841a21e557c1230e272bd46ab5b100000000"
	txBytes, err := hex.DecodeString(txHex)
	require.NoError(t, err)
	var stakingTx wire.MsgTx
	err = stakingTx.Deserialize(bytes.NewReader(txBytes))
	require.NoError(t, err)
	parsedData, err := btcstaking.ParseV0StakingTx(
		&stakingTx,
		p.Tag,
		p.CovenantPks,
		p.CovenantQuorum,
		&chaincfg.SigNetParams)
	require.NoError(t, err)
	t.Logf("parsed data: %d", parsedData.OpReturnData.StakingTime)
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

package indexer_test

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/babylonchain/babylon/btcstaking"
	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	"github.com/babylonchain/networks/parameters/parser"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/scalarorg/btcvault"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/btcscanner"
	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/indexer"
	"github.com/babylonchain/staking-indexer/testutils"
	"github.com/babylonchain/staking-indexer/testutils/datagen"
	"github.com/babylonchain/staking-indexer/types"
)

type VaultTxData struct {
	VaultTx   *btcutil.Tx
	VaultData *datagen.TestVaultData
	Burned    bool
}

type VaultEvent struct {
	VaultTx     *btcutil.Tx
	VaultTxData *datagen.TestVaultData
	Height      int32
	Burned      bool
	IsOverflow  bool
}

type BurningEvent struct {
	VaultTxHash *chainhash.Hash
	BurningTx   *btcutil.Tx
	Height      int32
}

type TestScenarioVault struct {
	VersionedParams *parser.ParsedGlobalParams
	VaultEvents     []*VaultEvent
	BurningEvents   []*BurningEvent
	Blocks          []*types.IndexedBlock
	TvlToHeight     map[int32]btcutil.Amount
	Tvl             btcutil.Amount
}

func NewTestScenarioVault(r *rand.Rand, t *testing.T, versionedParams *parser.ParsedGlobalParams, vaultChance int, numEvents int, checkOverflow bool) *TestScenarioVault {
	startHeight := r.Int31n(1000) + 1 + int32(versionedParams.Versions[0].ActivationHeight)
	lastEventHeight := startHeight
	vaultEvents := make([]*VaultEvent, 0)
	burningEvents := make([]*BurningEvent, 0)
	tvl := btcutil.Amount(0)
	txsPerHeight := make(map[int32][]*btcutil.Tx)
	tvlToHeight := make(map[int32]btcutil.Amount)

	// create numEvents events
	for i := 0; i < numEvents; i++ {
		// randomly select a height at which the event should be happening
		height := lastEventHeight + r.Int31n(3)
		p := versionedParams.GetVersionedGlobalParamsByHeight(uint64(height))
		require.NotNil(t, p)
		txs, ok := txsPerHeight[height]
		if !ok {
			// new height
			txs = make([]*btcutil.Tx, 0)
		}

		// stakingChance/100 chance to be a staking event, or there are
		// no active staking events created, otherwise, to be an unbonding event
		if r.Intn(100) < vaultChance || !hasActiveVaultEvent(vaultEvents) {
			vaultEvent := buildVaultEvent(r, t, height, p)
			if checkOverflow && isOverflow(uint64(height), tvl, p) {
				vaultEvent.IsOverflow = true
			} else {
				tvl += vaultEvent.VaultTxData.StakingAmount
			}
			vaultEvents = append(vaultEvents, vaultEvent)
			txs = append(txs, vaultEvent.VaultTx)
		} else {
			prevVaultEvent := findActiveVaultEvent(vaultEvents, r)
			require.NotNil(t, prevVaultEvent)
			require.False(t, prevVaultEvent.Burned)
			vaultParams := versionedParams.GetVersionedGlobalParamsByHeight(uint64(prevVaultEvent.Height))
			require.NotNil(t, vaultParams)
			burningEvent := buildBurningEvent(prevVaultEvent, height, vaultParams, t)
			burningEvents = append(burningEvents, burningEvent)
			prevVaultEvent.Burned = true
			if !prevVaultEvent.IsOverflow {
				tvl -= prevVaultEvent.VaultTxData.StakingAmount
			}
			require.True(t, tvl >= 0)
			txs = append(txs, burningEvent.BurningTx)
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

	return &TestScenarioVault{
		VersionedParams: versionedParams,
		VaultEvents:     vaultEvents,
		BurningEvents:   burningEvents,
		Blocks:          blocks,
		TvlToHeight:     tvlToHeight,
		Tvl:             tvl,
	}
}

func buildBurningEvent(vaultEvent *VaultEvent, height int32, p *parser.ParsedVersionedGlobalParams, t *testing.T) *BurningEvent {
	vaultTxHash := vaultEvent.VaultTx.Hash()
	burningTx := datagen.GenerateBurningTxFromVault(t, p, vaultEvent.VaultTxData, vaultTxHash, 0)

	return &BurningEvent{
		VaultTxHash: vaultTxHash,
		BurningTx:   burningTx,
		Height:      height,
	}
}

func findActiveVaultEvent(vaultEvents []*VaultEvent, r *rand.Rand) *VaultEvent {
	var activeVaultEvents []*VaultEvent
	for _, va := range vaultEvents {
		if !va.Burned {
			activeVaultEvents = append(activeVaultEvents, va)
		}
	}
	if len(activeVaultEvents) == 0 {
		return nil
	}
	return activeVaultEvents[r.Intn(len(activeVaultEvents))]
}

func hasActiveVaultEvent(vaultEvents []*VaultEvent) bool {
	for _, va := range vaultEvents {
		if !va.Burned {
			return true
		}
	}

	return false
}

func buildVaultEvent(r *rand.Rand, t *testing.T, height int32, p *parser.ParsedVersionedGlobalParams) *VaultEvent {
	vaultData := datagen.GenerateTestVaultData(t, r, p)
	_, vaultTx := datagen.GenerateVaultTxFromTestData(t, r, p, vaultData)

	return &VaultEvent{
		VaultTx:     vaultTx,
		VaultTxData: vaultData,
		Height:      height,
	}
}

func FuzzBlockHandlerScalar(f *testing.F) {
	// Note: before committing, it should be tested with large seed
	// to avoid flaky
	// small seed for ci because db open/close is slow
	bbndatagen.AddRandomSeedsToFuzzer(f, 50)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		homePath := filepath.Join(t.TempDir(), "indexer")
		cfg := config.DefaultConfigWithHome(homePath)

		n := r.Intn(100) + 1
		sysParamsVersions := datagen.GenerateGlobalParamsVersions(r, t)
		testScenario := NewTestScenarioVault(r, t, sysParamsVersions, 80, n, true)

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
			err := stakingIndexer.HandleConfirmedBlockScalar(b)
			require.NoError(t, err)
			tvl, err := stakingIndexer.GetConfirmedTvl()
			require.NoError(t, err)
			require.Equal(t, uint64(testScenario.TvlToHeight[b.Height]), tvl)
		}
		tvl, err := stakingIndexer.GetConfirmedTvl()
		require.NoError(t, err)
		require.Equal(t, uint64(testScenario.Tvl), tvl)

		for _, vaultEv := range testScenario.VaultEvents {
			storedTx, err := stakingIndexer.GetVaultTxByHash(vaultEv.VaultTx.Hash())
			require.NoError(t, err)
			require.NotNil(t, storedTx)
			require.Equal(t, vaultEv.VaultTx.Hash().String(), storedTx.Tx.TxHash().String())
			require.True(t, testutils.PubKeysEqual(vaultEv.VaultTxData.StakerKey, storedTx.StakerPk))
			require.True(t, testutils.PubKeysEqual(vaultEv.VaultTxData.FinalityProviderKey, storedTx.DAppPk))
			require.Equal(t, vaultEv.IsOverflow, storedTx.IsOverflow)
		}

		for _, burningEv := range testScenario.BurningEvents {
			storedTx, err := stakingIndexer.GetBurningTxByHash(burningEv.BurningTx.Hash())
			require.NoError(t, err)
			require.NotNil(t, storedTx)
			require.Equal(t, burningEv.VaultTxHash, storedTx.VaultTxHash)
			require.Equal(t, burningEv.BurningTx.Hash().String(), storedTx.Tx.TxHash().String())
		}

		// replay the blocks and the result should be the same
		for _, b := range testScenario.Blocks {
			err := stakingIndexer.HandleConfirmedBlockScalar(b)
			require.NoError(t, err)
		}
		tvl, err = stakingIndexer.GetConfirmedTvl()
		require.NoError(t, err)
		require.Equal(t, uint64(testScenario.Tvl), tvl)

		for _, vaultEv := range testScenario.VaultEvents {
			storedTx, err := stakingIndexer.GetVaultTxByHash(vaultEv.VaultTx.Hash())
			require.NoError(t, err)
			require.NotNil(t, storedTx)
			require.Equal(t, vaultEv.VaultTx.Hash().String(), storedTx.Tx.TxHash().String())
			require.True(t, testutils.PubKeysEqual(vaultEv.VaultTxData.StakerKey, storedTx.StakerPk))
			require.True(t, testutils.PubKeysEqual(vaultEv.VaultTxData.FinalityProviderKey, storedTx.DAppPk))
			require.Equal(t, vaultEv.IsOverflow, storedTx.IsOverflow)
		}

		for _, burningEv := range testScenario.BurningEvents {
			storedTx, err := stakingIndexer.GetBurningTxByHash(burningEv.BurningTx.Hash())
			require.NoError(t, err)
			require.NotNil(t, storedTx)
			require.Equal(t, burningEv.VaultTxHash, storedTx.VaultTxHash)
			require.Equal(t, burningEv.BurningTx.Hash().String(), storedTx.Tx.TxHash().String())
		}

		// calculate unconfirmed tvl
		testUnconfirmedScenario := NewTestScenarioVault(r, t, sysParamsVersions, 80, n, false)
		unconfirmedTvl, err := stakingIndexer.CalculateTvlInUnconfirmedBlocksScalar(testUnconfirmedScenario.Blocks)
		require.NoError(t, err)
		require.Equal(t, testUnconfirmedScenario.Tvl, unconfirmedTvl)
	})
}

// FuzzVerifyBurningTx tests IsValidBurningTx in three scenarios:
// 1. it returns (true, nil) if the given tx is valid burning tx
// 2. it returns (false, nil) if the given tx is not burning tx
// 3. it returns (false, ErrInvalidBurningTx) if the given tx is not
// a valid burning tx
func FuzzVerifyBurningTx(f *testing.F) {
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
		params := sysParamsVersions.Versions[0]
		// 1. generate and add a valid staking tx to the indexer
		vaultData := datagen.GenerateTestVaultData(t, r, params)
		_, vaultTx := datagen.GenerateVaultTxFromTestData(t, r, params, vaultData)
		// For a valid tx, its btc height is always larger than the activation height
		mockedHeight := uint64(params.ActivationHeight) + 1
		err = stakingIndexer.ProcessVaultTx(
			vaultTx.MsgTx(),
			getParsedVaultData(vaultData, vaultTx.MsgTx(), params),
			mockedHeight, time.Now(), params)
		require.NoError(t, err)
		storedVaultTx, err := stakingIndexer.GetVaultTxByHash(vaultTx.Hash())
		require.NoError(t, err)
		require.NotNil(t, storedVaultTx)

		// 2. test IsValidBurningTx with valid burning tx, expect (true, nil)
		burningTx := datagen.GenerateBurningTxFromVault(t, params, vaultData, vaultTx.Hash(), 0)
		isValid, err := stakingIndexer.IsValidBurningTx(burningTx.MsgTx(), storedVaultTx, params)
		require.NoError(t, err)
		require.True(t, isValid)

		// 3. test IsValidBurningTx with no burning tx (different staking output index), expect (false, nil)
		burningTx = datagen.GenerateBurningTxFromVault(t, params, vaultData, vaultTx.Hash(), 1)
		isValid, err = stakingIndexer.IsValidBurningTx(burningTx.MsgTx(), storedVaultTx, params)
		require.NoError(t, err)
		require.False(t, isValid)

		// 4. test IsValidUnbondingTx with rbf enabled, expect (false, ErrInvalidBurningTx)
		burningTx = datagen.GenerateBurningTxFromVault(t, params, vaultData, vaultTx.Hash(), 0)
		burningTx.MsgTx().TxIn[0].Sequence = 0
		isValid, err = stakingIndexer.IsValidBurningTx(burningTx.MsgTx(), storedVaultTx, params)
		require.ErrorIs(t, err, indexer.ErrInvalidBurningTx)
		require.Contains(t, err.Error(), "burning tx should not enable rbf")
		require.False(t, isValid)

		// 5. test IsValidBurningTx with time lock set, expect (false, ErrInvalidBurningTx)
		burningTx = datagen.GenerateBurningTxFromVault(t, params, vaultData, vaultTx.Hash(), 0)
		burningTx.MsgTx().LockTime = 1
		isValid, err = stakingIndexer.IsValidBurningTx(burningTx.MsgTx(), storedVaultTx, params)
		require.ErrorIs(t, err, indexer.ErrInvalidBurningTx)
		require.Contains(t, err.Error(), "burning tx should not set lock time")
		require.False(t, isValid)

		// 6. test IsValidBurningTx with invalid burning tx (random burning fee in params), expect (false, ErrInvalidBurningTx)
		newParams := *params
		newParams.UnbondingFee = btcutil.Amount(bbndatagen.RandomIntOtherThan(r, int(params.UnbondingFee), 10000000))
		burningTx = datagen.GenerateBurningTxFromVault(t, &newParams, vaultData, vaultTx.Hash(), 0)
		// pass the old params
		isValid, err = stakingIndexer.IsValidBurningTx(burningTx.MsgTx(), storedVaultTx, params)
		require.ErrorIs(t, err, indexer.ErrInvalidBurningTx)
		require.Contains(t, err.Error(), fmt.Sprintf("the burning output value %d is not expected", burningTx.MsgTx().TxOut[0].Value))
		require.False(t, isValid)
	})
}

func FuzzValidateWithdrawTxFromVault(f *testing.F) {
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
		params := sysParamsVersions.Versions[0]
		// 1. generate and add a valid staking tx to the indexer
		vaultData := datagen.GenerateTestVaultData(t, r, params)
		_, vaultTx := datagen.GenerateVaultTxFromTestData(t, r, params, vaultData)
		// For a valid tx, its btc height is always larger than the activation height
		mockedHeight := uint64(params.ActivationHeight) + 1
		err = stakingIndexer.ProcessVaultTx(
			vaultTx.MsgTx(),
			getParsedVaultData(vaultData, vaultTx.MsgTx(), params),
			mockedHeight, time.Now(), params)
		require.NoError(t, err)
		storedVaultTx, err := stakingIndexer.GetVaultTxByHash(vaultTx.Hash())
		require.NoError(t, err)
		require.NotNil(t, storedVaultTx)

		// 2. test ValidateWithdrawalTxFromVault with valid withdrawal tx
		withdrawTxFromVault := datagen.GenerateWithdrawalTxFromVault(t, r, params, vaultData, vaultTx.Hash(), 0)
		err = stakingIndexer.ValidateWithdrawalTxFromVault(withdrawTxFromVault.MsgTx(), storedVaultTx, 0, params)
		require.NoError(t, err)

		// 3. test ValidateWithdrawalTxFromVault with invalid spending input index, expect panic
		require.Panics(t, func() {
			_ = stakingIndexer.ValidateWithdrawalTxFromVault(withdrawTxFromVault.MsgTx(), storedVaultTx, 1, params)
		})
	})
}

func FuzzValidateWithdrawTxFromBurning(f *testing.F) {
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
		params := sysParamsVersions.Versions[0]
		// 1. generate and add a valid vault tx to the indexer
		vaultData := datagen.GenerateTestVaultData(t, r, params)
		_, vaultTx := datagen.GenerateVaultTxFromTestData(t, r, params, vaultData)
		// For a valid tx, its btc height is always larger than the activation height
		mockedHeight := uint64(params.ActivationHeight) + 1
		err = stakingIndexer.ProcessVaultTx(
			vaultTx.MsgTx(),
			getParsedVaultData(vaultData, vaultTx.MsgTx(), params),
			mockedHeight, time.Now(), params)
		require.NoError(t, err)
		storedVaultTx, err := stakingIndexer.GetVaultTxByHash(vaultTx.Hash())
		require.NoError(t, err)
		require.NotNil(t, storedVaultTx)

		// 2. generate a valid burning tx
		burningTx := datagen.GenerateBurningTxFromVault(t, params, vaultData, vaultTx.Hash(), 0)
		isValid, err := stakingIndexer.IsValidBurningTx(burningTx.MsgTx(), storedVaultTx, params)
		require.NoError(t, err)
		require.True(t, isValid)

		// 3. test ValidateWithdrawalTxFromBurning with valid withdrawal tx
		withdrawTxFromBurning := datagen.GenerateWithdrawalTxFromBurning(t, r, params, vaultData, vaultTx.Hash())
		err = stakingIndexer.ValidateWithdrawalTxFromBurning(withdrawTxFromBurning.MsgTx(), storedVaultTx, 0, params)
		require.NoError(t, err)

		// 4. test ValidateWithdrawalTxFromBurning with invalid spending input index, expect panic
		require.Panics(t, func() {
			_ = stakingIndexer.ValidateWithdrawalTxFromBurning(withdrawTxFromBurning.MsgTx(), storedVaultTx, 1, params)
		})
	})
}

func getParsedVaultData(data *datagen.TestVaultData, tx *wire.MsgTx, params *parser.ParsedVersionedGlobalParams) *btcvault.ParsedV0VaultTx {
	return &btcvault.ParsedV0VaultTx{
		VaultOutput:       tx.TxOut[0],
		VaultOutputIdx:    0,
		OpReturnOutput:    tx.TxOut[1],
		OpReturnOutputIdx: 1,
		OpReturnData: &btcstaking.V0OpReturnData{
			Tag:                       params.Tag,
			Version:                   0,
			StakerPublicKey:           &btcstaking.XonlyPubKey{PubKey: data.StakerKey},
			FinalityProviderPublicKey: &btcstaking.XonlyPubKey{PubKey: data.FinalityProviderKey},
		},
		PayloadOutput:    tx.TxOut[2],
		PayloadOutputIdx: 2,
		PayloadOpReturnData: &btcvault.PayloadOpReturnData{
			ChainID:                     data.ChainID,
			ChainIdUserAddress:          data.ChainIdUserAddress,
			ChainIdSmartContractAddress: data.ChainIdSmartContractAddress,
			Amount:                      data.MintingAmount,
		},
	}
}

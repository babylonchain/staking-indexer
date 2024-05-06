package indexerstore_test

import (
	"errors"
	"math/rand"
	"testing"
	"time"

	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/indexerstore"
	"github.com/babylonchain/staking-indexer/testutils"
	"github.com/babylonchain/staking-indexer/testutils/datagen"
)

func TestEmptyStore(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	db := testutils.MakeTestBackend(t)
	s, err := indexerstore.NewIndexerStore(db)
	require.NoError(t, err)
	hash := bbndatagen.GenRandomBtcdHash(r)
	stakingTx, err := s.GetStakingTransaction(&hash)
	require.Nil(t, stakingTx)
	require.Error(t, err)
	require.True(t, errors.Is(err, indexerstore.ErrTransactionNotFound))
	unbondingTx, err := s.GetUnbondingTransaction(&hash)
	require.Nil(t, unbondingTx)
	require.Error(t, err)
	require.True(t, errors.Is(err, indexerstore.ErrTransactionNotFound))
}

func FuzzStoringTxs(f *testing.F) {
	// only 3 seeds as this is pretty slow test opening/closing db
	bbndatagen.AddRandomSeedsToFuzzer(f, 3)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		db := testutils.MakeTestBackend(t)
		s, err := indexerstore.NewIndexerStore(db)
		require.NoError(t, err)
		maxCreatedTx := 30
		numTx := r.Intn(maxCreatedTx) + 1
		stakingtxs := datagen.GenNStoredStakingTxs(t, r, numTx, 200)

		// add staking txs to store
		for _, storedTx := range stakingtxs {
			err := s.AddStakingTransaction(
				storedTx.Tx,
				storedTx.StakingOutputIdx,
				storedTx.InclusionHeight,
				storedTx.StakerPk,
				storedTx.StakingTime,
				storedTx.FinalityProviderPk,
				storedTx.StakingValue,
				storedTx.IsOverflow,
			)
			require.NoError(t, err)
		}

		// check staking txs from store
		for _, storedTx := range stakingtxs {
			hash := storedTx.Tx.TxHash()
			tx, err := s.GetStakingTransaction(&hash)
			require.NoError(t, err)
			require.Equal(t, storedTx.Tx, tx.Tx)
			require.True(t, testutils.PubKeysEqual(storedTx.StakerPk, tx.StakerPk))
			require.Equal(t, storedTx.StakingTime, tx.StakingTime)
			require.True(t, testutils.PubKeysEqual(storedTx.FinalityProviderPk, tx.FinalityProviderPk))
		}

		// add unbonding txs to store
		unbondingTxs := datagen.GenStoredUnbondingTxs(r, stakingtxs)
		for _, storedTx := range unbondingTxs {
			err := s.AddUnbondingTransaction(storedTx.Tx, storedTx.StakingTxHash)
			require.NoError(t, err)
		}

		// check unbonding txs from store
		for _, storedTx := range unbondingTxs {
			hash := storedTx.Tx.TxHash()
			tx, err := s.GetUnbondingTransaction(&hash)
			require.NoError(t, err)
			require.Equal(t, storedTx.Tx, tx.Tx)
			require.True(t, storedTx.StakingTxHash.IsEqual(tx.StakingTxHash))
		}

		// add unbonding txs that do not spend previous staking tx
		// should expect error
		// add unbonding txs to store
		notStoredStakingTxs := datagen.GenNStoredStakingTxs(t, r, numTx, 200)
		wrongUnbondingTxs := datagen.GenStoredUnbondingTxs(r, notStoredStakingTxs)
		for _, storedTx := range wrongUnbondingTxs {
			err := s.AddUnbondingTransaction(storedTx.Tx, storedTx.StakingTxHash)
			require.ErrorIs(t, err, indexerstore.ErrTransactionNotFound)
		}
	})
}

func FuzzStoringIndexerState(f *testing.F) {
	// only 3 seeds as this is pretty slow test opening/closing db
	bbndatagen.AddRandomSeedsToFuzzer(f, 3)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		db := testutils.MakeTestBackend(t)
		s, err := indexerstore.NewIndexerStore(db)
		require.NoError(t, err)

		_, err = s.GetLastProcessedHeight()
		require.ErrorIs(t, err, indexerstore.ErrLastProcessedHeightNotFound)

		lastProcessedHeight := uint64(r.Int63n(1000) + 1)
		err = s.SaveLastProcessedHeight(lastProcessedHeight)
		require.NoError(t, err)

		storedLastProcessedHeight, err := s.GetLastProcessedHeight()
		require.NoError(t, err)
		require.Equal(t, lastProcessedHeight, storedLastProcessedHeight)
	})
}

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
	tx, err := s.GetStakingTransaction(&hash)
	require.Nil(t, tx)
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
		generatedStoredTxs := datagen.GenNStoredStakingTxs(t, r, numTx, 200)
		for _, storedTx := range generatedStoredTxs {
			err := s.AddStakingTransaction(
				storedTx.Tx,
				storedTx.StakingOutputIdx,
				storedTx.InclusionHeight,
				storedTx.StakerPk,
				storedTx.StakingTime,
				storedTx.FinalityProviderPk,
			)
			require.NoError(t, err)
		}
		for _, storedTx := range generatedStoredTxs {
			hash := storedTx.Tx.TxHash()
			tx, err := s.GetStakingTransaction(&hash)
			require.NoError(t, err)
			require.Equal(t, storedTx.Tx, tx.Tx)
			require.True(t, testutils.PubKeysEqual(storedTx.StakerPk, tx.StakerPk))
			require.Equal(t, storedTx.StakingTime, tx.StakingTime)
			require.True(t, testutils.PubKeysEqual(storedTx.FinalityProviderPk, tx.FinalityProviderPk))
		}

	})
}

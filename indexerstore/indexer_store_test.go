package indexerstore_test

import (
	"bytes"
	"errors"
	"math/rand"
	"testing"
	"time"

	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/indexerstore"
	"github.com/babylonchain/staking-indexer/testutils/datagen"
)

func MakeTestStore(t *testing.T) *indexerstore.IndexerStore {
	// First, create a temporary directory to be used for the duration of
	// this test.
	tempDirName := t.TempDir()

	cfg := config.DefaultDBConfig()

	cfg.DBPath = tempDirName

	backend, err := cfg.GetDbBackend()
	require.NoError(t, err)

	t.Cleanup(func() {
		backend.Close()
	})

	store, err := indexerstore.NewIndexerStore(backend)
	require.NoError(t, err)

	return store
}

func TestEmptyStore(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	s := MakeTestStore(t)
	hash := bbndatagen.GenRandomBtcdHash(r)
	tx, err := s.GetTransaction(&hash)
	require.Nil(t, tx)
	require.Error(t, err)
	require.True(t, errors.Is(err, indexerstore.ErrTransactionNotFound))
}

func FuzzStoringTxs(f *testing.F) {
	// only 3 seeds as this is pretty slow test opening/closing db
	bbndatagen.AddRandomSeedsToFuzzer(f, 3)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		s := MakeTestStore(t)
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
				storedTx.FinalityProviderPks,
			)
			require.NoError(t, err)
		}
		for _, storedTx := range generatedStoredTxs {
			hash := storedTx.Tx.TxHash()
			tx, err := s.GetTransaction(&hash)
			require.NoError(t, err)
			require.Equal(t, storedTx.Tx, tx.Tx)
			require.True(t, pubKeysEqual(storedTx.StakerPk, tx.StakerPk))
			require.Equal(t, storedTx.StakingTime, tx.StakingTime)
			require.True(t, pubKeysSliceEqual(storedTx.FinalityProviderPks, tx.FinalityProviderPks))
		}

	})
}

func pubKeysSliceEqual(pk1, pk2 []*btcec.PublicKey) bool {
	if len(pk1) != len(pk2) {
		return false
	}

	for i := 0; i < len(pk1); i++ {
		if !pubKeysEqual(pk1[i], pk2[i]) {
			return false
		}
	}

	return true
}

func pubKeysEqual(pk1, pk2 *btcec.PublicKey) bool {
	return bytes.Equal(schnorr.SerializePubKey(pk1), schnorr.SerializePubKey(pk2))
}

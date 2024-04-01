package datagen

import (
	"math/rand"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/types"
)

// GenerateGlobalParams generate test params and save it in a file
// It returns the file path
func GenerateGlobalParams(r *rand.Rand, t *testing.T) *types.Params {
	tag := "bbt4"

	numCovenants := r.Intn(10) + 1
	covPks := make([]*btcec.PublicKey, numCovenants)
	for i := 0; i < numCovenants; i++ {
		privKey, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		covPks[i] = privKey.PubKey()
	}

	covQuorum := uint32(r.Intn(numCovenants) + 1)

	unbondingTime := uint16(r.Intn(1000) + 100)

	minStakingAmount := btcutil.Amount(r.Int63n(10000) + 1)

	maxStakingAmount := btcutil.Amount(r.Int63n(1000000)) + minStakingAmount

	minStakingTime := uint16(r.Intn(1000)) + 1

	maxStakingTime := uint16(r.Intn(10000)) + minStakingTime

	return &types.Params{
		Tag:              []byte(tag),
		CovenantPks:      covPks,
		CovenantQuorum:   covQuorum,
		UnbondingTime:    unbondingTime,
		MaxStakingAmount: maxStakingAmount,
		MinStakingAmount: minStakingAmount,
		MaxStakingTime:   maxStakingTime,
		MinStakingTime:   minStakingTime,
	}
}

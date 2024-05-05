package datagen

import (
	"math/rand"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/types"
)

// GenerateGlobalParamsVersions generate test params and save it in a file
// It returns the file path
func GenerateGlobalParamsVersions(r *rand.Rand, t *testing.T) *types.ParamsVersions {
	// Random number of versions
	numVersions := uint16(r.Intn(10) + 1)

	// For now keep the same covenants across versions
	// TODO: consider covenants updates here
	numCovenants := r.Intn(10) + 1
	covPks := make([]*btcec.PublicKey, numCovenants)
	for i := 0; i < numCovenants; i++ {
		privKey, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		covPks[i] = privKey.PubKey()
	}
	covQuorum := uint32(r.Intn(numCovenants) + 1)

	// Keep the same tag across versions
	// TODO: consider tag updates
	tag := "bbt4"

	paramsVersions := &types.ParamsVersions{
		ParamsVersions: make([]*types.Params, 0),
	}
	lastStakingCap := btcutil.Amount(0)
	lastActivationHeight := int32(0)
	for version := uint16(0); version <= numVersions; version++ {
		// These parameters can freely change between versions
		unbondingTime := uint16(r.Intn(1000) + 100)
		unbondingFee := btcutil.Amount(r.Int63n(10000) + 1)
		// Min Staking Amount should be more than the required unbonding fee
		minStakingAmount := btcutil.Amount(r.Int63n(100000)+1) + unbondingFee
		// Max Staking Amount should be more than the minimum staking amount
		maxStakingAmount := btcutil.Amount(r.Int63n(1000000)) + minStakingAmount
		minStakingTime := uint16(r.Intn(1000)) + 1
		maxStakingTime := uint16(r.Intn(10000)) + minStakingTime

		// These parameters should be monotonically increasing
		// The staking cap should be more than the maximum staking amount
		stakingCap := btcutil.Amount(r.Int63n(1000000)) + maxStakingAmount + lastStakingCap
		lastStakingCap = stakingCap
		var activationHeight int32
		if version == 0 {
			activationHeight = 1
		} else {
			activationHeight = int32(r.Intn(1000)) + lastActivationHeight + 1
		}
		lastActivationHeight = activationHeight
		paramsVersions.ParamsVersions = append(paramsVersions.ParamsVersions, &types.Params{
			Version:          version,
			StakingCap:       stakingCap,
			ActivationHeight: activationHeight,
			Tag:              []byte(tag),
			CovenantPks:      covPks,
			CovenantQuorum:   covQuorum,
			UnbondingTime:    unbondingTime,
			UnbondingFee:     unbondingFee,
			MaxStakingAmount: maxStakingAmount,
			MinStakingAmount: minStakingAmount,
			MaxStakingTime:   maxStakingTime,
			MinStakingTime:   minStakingTime,
		})
	}

	return paramsVersions
}

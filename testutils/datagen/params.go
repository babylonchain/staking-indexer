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
	tag := []byte{0x01, 0x02, 0x03, 0x04}

	paramsVersions := &types.ParamsVersions{
		ParamsVersions: make([]*types.GlobalParams, 0),
	}
	lastStakingCap := btcutil.Amount(0)
	lastActivationHeight := int32(0)
	lastCovKeys := make([]*btcec.PublicKey, numCovenants)
	// confirmation depth is at least 2
	confirmationDepth := uint16(r.Intn(100) + 2)
	copy(lastCovKeys, covPks)
	for version := uint16(0); version <= numVersions; version++ {
		// These parameters can freely change between versions
		unbondingTime := uint16(r.Intn(1000) + 100)
		unbondingFee := btcutil.Amount(r.Int63n(10000) + 1)
		// Min Staking Amount should be more than the required unbonding fee
		minStakingAmount := btcutil.Amount(r.Int63n(100000)+1) + unbondingFee
		// Max Staking Amount should be more than the minimum staking amount
		maxStakingAmount := btcutil.Amount(r.Int63n(100000)) + minStakingAmount
		minStakingTime := uint16(r.Intn(1000)) + 1
		maxStakingTime := uint16(r.Intn(10000)) + minStakingTime

		// These parameters should be monotonically increasing
		// The staking cap should be more than the maximum staking amount
		stakingCap := btcutil.Amount(r.Int63n(1000000)) +
			btcutil.Amount(r.Int63n(100000)) + maxStakingAmount + lastStakingCap
		lastStakingCap = stakingCap
		activationHeight := int32(r.Intn(100)) + lastActivationHeight + 1
		lastActivationHeight = activationHeight
		rotatedKeys := rotateCovenantPks(lastCovKeys, r, t)
		copy(lastCovKeys, rotatedKeys)
		paramsVersions.ParamsVersions = append(paramsVersions.ParamsVersions, &types.GlobalParams{
			Version:           version,
			StakingCap:        stakingCap,
			ActivationHeight:  uint64(activationHeight),
			Tag:               tag,
			CovenantPks:       rotatedKeys,
			CovenantQuorum:    covQuorum,
			UnbondingTime:     unbondingTime,
			UnbondingFee:      unbondingFee,
			MaxStakingAmount:  maxStakingAmount,
			MinStakingAmount:  minStakingAmount,
			MaxStakingTime:    maxStakingTime,
			MinStakingTime:    minStakingTime,
			ConfirmationDepth: confirmationDepth,
		})
	}

	return paramsVersions
}

// rotateCovenantPks randomly rotates max 2 public keys and returns a new list of keys
func rotateCovenantPks(oldKeys []*btcec.PublicKey, r *rand.Rand, t *testing.T) []*btcec.PublicKey {
	newKeys := make([]*btcec.PublicKey, len(oldKeys))
	copy(newKeys, oldKeys)
	randIndex := r.Intn(len(oldKeys))
	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	newKeys[randIndex] = privKey.PubKey()

	if len(newKeys) == 1 {
		return newKeys
	}

	// if the covenant key number > 1, do one more round
	randIndex = r.Intn(len(newKeys))
	privKey, err = btcec.NewPrivateKey()
	require.NoError(t, err)
	newKeys[randIndex] = privKey.PubKey()

	return newKeys
}

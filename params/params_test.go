package params_test

import (
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/params"
	"github.com/babylonchain/staking-indexer/testutils"
	"github.com/babylonchain/staking-indexer/testutils/datagen"
	"github.com/babylonchain/staking-indexer/types"
)

type internalParamsVersions struct {
	ParamsVersions []*internalParams `json:"versions"`
}

type internalParams struct {
	Version           uint16         `json:"version"`
	ActivationHeight  int32          `json:"activation_height"`
	StakingCap        btcutil.Amount `json:"staking_cap"`
	Tag               string         `json:"tag"`
	CovenantPks       []string       `json:"covenant_pks"`
	CovenantQuorum    uint32         `json:"covenant_quorum"`
	UnbondingTime     uint16         `json:"unbonding_time"`
	UnbondingFee      btcutil.Amount `json:"unbonding_fee"`
	MaxStakingAmount  btcutil.Amount `json:"max_staking_amount"`
	MinStakingAmount  btcutil.Amount `json:"min_staking_amount"`
	MaxStakingTime    uint16         `json:"max_staking_time"`
	MinStakingTime    uint16         `json:"min_staking_time"`
	ConfirmationDepth uint16         `json:"confirmation_depth"`
}

func FuzzParamsRetriever(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		// generate global params
		globalParamsVersions := datagen.GenerateGlobalParamsVersions(r, t)

		jsonBytes, err := json.MarshalIndent(paramsToInternalParams(globalParamsVersions), "", "    ")
		require.NoError(t, err)

		// write params to file
		path := filepath.Join(t.TempDir(), "test-params.json")
		err = os.WriteFile(path, jsonBytes, os.ModePerm)
		defer os.Remove(path)
		require.NoError(t, err)

		// the params retriever read the file
		paramsRetriever, err := params.NewLocalParamsRetriever(path)
		require.NoError(t, err)
		pv := paramsRetriever.GetParamsVersions()

		// check the values are expected
		require.Equal(t, len(globalParamsVersions.ParamsVersions), len(pv.ParamsVersions))
		for idx, globalParams := range globalParamsVersions.ParamsVersions {
			p := pv.ParamsVersions[idx]
			require.Equal(t, globalParams.Version, p.Version)
			require.Equal(t, globalParams.ActivationHeight, p.ActivationHeight)
			require.Equal(t, globalParams.StakingCap, p.StakingCap)
			require.Equal(t, globalParams.Tag, p.Tag)
			require.Equal(t, globalParams.MinStakingTime, p.MinStakingTime)
			require.Equal(t, globalParams.MaxStakingTime, p.MaxStakingTime)
			require.Equal(t, globalParams.MinStakingAmount, p.MinStakingAmount)
			require.Equal(t, globalParams.MaxStakingAmount, p.MaxStakingAmount)
			require.Equal(t, globalParams.ConfirmationDepth, p.ConfirmationDepth)
			require.Equal(t, globalParams.CovenantQuorum, p.CovenantQuorum)
			require.Equal(t, globalParams.UnbondingTime, p.UnbondingTime)
			require.True(t, testutils.PubKeysSliceEqual(globalParams.CovenantPks, p.CovenantPks))
		}
	})
}

func paramsToInternalParams(pv *types.ParamsVersions) *internalParamsVersions {
	paramsVersions := &internalParamsVersions{
		ParamsVersions: make([]*internalParams, 0),
	}
	for _, p := range pv.ParamsVersions {
		covPksHex := make([]string, len(p.CovenantPks))
		for i, pk := range p.CovenantPks {
			covPksHex[i] = hex.EncodeToString(pk.SerializeCompressed())
		}

		paramsVersions.ParamsVersions = append(paramsVersions.ParamsVersions, &internalParams{
			Version:           p.Version,
			ActivationHeight:  p.ActivationHeight,
			StakingCap:        p.StakingCap,
			Tag:               hex.EncodeToString(p.Tag),
			CovenantPks:       covPksHex,
			CovenantQuorum:    p.CovenantQuorum,
			UnbondingTime:     p.UnbondingTime,
			MaxStakingAmount:  p.MaxStakingAmount,
			MinStakingAmount:  p.MinStakingAmount,
			MaxStakingTime:    p.MaxStakingTime,
			MinStakingTime:    p.MinStakingTime,
			ConfirmationDepth: p.ConfirmationDepth,
		})
	}
	return paramsVersions
}

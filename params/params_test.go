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

type internalParams struct {
	Tag              string         `json:"tag"`
	CovenantPks      []string       `json:"covenant_pks"`
	CovenantQuorum   uint32         `json:"covenant_quorum"`
	UnbondingTime    uint16         `json:"unbonding_time"`
	UnbondingFee     btcutil.Amount `json:"unbonding_fee"`
	MaxStakingAmount btcutil.Amount `json:"max_staking_amount"`
	MinStakingAmount btcutil.Amount `json:"min_staking_amount"`
	MaxStakingTime   uint16         `json:"max_staking_time"`
	MinStakingTime   uint16         `json:"min_staking_time"`
}

func FuzzParamsRetriever(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		// generate global params
		globalParams := datagen.GenerateGlobalParams(r, t)

		jsonBytes, err := json.MarshalIndent(paramsToInternalParams(globalParams), "", "    ")
		require.NoError(t, err)

		// write params to file
		path := filepath.Join(t.TempDir(), "test-params.json")
		err = os.WriteFile(path, jsonBytes, os.ModePerm)
		defer os.Remove(path)
		require.NoError(t, err)

		// the params retriever read the file
		paramsRetriever, err := params.NewLocalParamsRetriever(path)
		require.NoError(t, err)
		p := paramsRetriever.GetParams()

		// check the values are expected
		require.Equal(t, globalParams.Tag, p.Tag)
		require.Equal(t, globalParams.MinStakingTime, p.MinStakingTime)
		require.Equal(t, globalParams.MaxStakingTime, p.MaxStakingTime)
		require.Equal(t, globalParams.MinStakingAmount, p.MinStakingAmount)
		require.Equal(t, globalParams.MaxStakingAmount, p.MaxStakingAmount)
		require.Equal(t, globalParams.CovenantQuorum, p.CovenantQuorum)
		require.Equal(t, globalParams.UnbondingTime, p.UnbondingTime)
		require.True(t, testutils.PubKeysSliceEqual(globalParams.CovenantPks, p.CovenantPks))
	})
}

func paramsToInternalParams(p *types.Params) *internalParams {
	covPksHex := make([]string, len(p.CovenantPks))
	for i, pk := range p.CovenantPks {
		covPksHex[i] = hex.EncodeToString(pk.SerializeCompressed())
	}

	return &internalParams{
		Tag:              string(p.Tag),
		CovenantPks:      covPksHex,
		CovenantQuorum:   p.CovenantQuorum,
		UnbondingTime:    p.UnbondingTime,
		MaxStakingAmount: p.MaxStakingAmount,
		MinStakingAmount: p.MinStakingAmount,
		MaxStakingTime:   p.MaxStakingTime,
		MinStakingTime:   p.MinStakingTime,
	}
}

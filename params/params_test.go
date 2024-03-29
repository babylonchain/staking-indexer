package params_test

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/params"
	"github.com/babylonchain/staking-indexer/testutils"
	"github.com/babylonchain/staking-indexer/testutils/datagen"
)

func FuzzParamsRetriever(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		// generate global params
		globalParams := datagen.GenerateGlobalParams(r, t)

		jsonBytes, err := json.MarshalIndent(globalParams.ToProto(), "", "    ")
		require.NoError(t, err)

		// write params to file
		path := filepath.Join(t.TempDir(), "test_params.json")
		err = os.WriteFile(path, jsonBytes, os.ModePerm)
		defer os.Remove(path)
		require.NoError(t, err)

		// the params retriever read the file
		paramsRetriever, err := params.NewLocalParamsRetriever(path)
		require.NoError(t, err)
		p := paramsRetriever.GetParams()

		// check the values are expected
		require.True(t, testutils.PubKeysSliceEqual(globalParams.CovenantPks, p.CovenantPks))
	})
}

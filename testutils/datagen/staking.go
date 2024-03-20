package datagen

import (
	"math"
	"math/rand"
	"testing"

	"github.com/babylonchain/babylon/btcstaking"
	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
)

type TestStakingParams struct {
	CovenantKeys   []*btcec.PublicKey
	CovenantQuorum uint32
	MagicBytes     []byte
	Net            *chaincfg.Params
}

type TestStakingData struct {
	StakerKey            *btcec.PublicKey
	FinalityProviderKeys []*btcec.PublicKey
	StakingAmount        btcutil.Amount
	StakingTime          uint16
}

func GenerateTestStakingParams(
	t *testing.T,
	r *rand.Rand,
	numCovenantKeys int,
	quorum uint32,
) *TestStakingParams {
	covenantKeys := make([]*btcec.PublicKey, numCovenantKeys)
	for i := 0; i < numCovenantKeys; i++ {
		covenantPrivKey, err := btcec.NewPrivateKey()
		require.NoError(t, err)

		covenantKeys[i] = covenantPrivKey.PubKey()
	}
	magicBytes := bbndatagen.GenRandomByteArray(r, btcstaking.MagicBytesLen)

	return &TestStakingParams{
		CovenantKeys:   covenantKeys,
		CovenantQuorum: quorum,
		MagicBytes:     magicBytes,
		Net:            &chaincfg.SimNetParams,
	}
}

func GenerateTestStakingData(
	t *testing.T,
	r *rand.Rand,
	numFinalityProvider int,
) *TestStakingData {
	stakerPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	finalityProviderKeys := make([]*btcec.PublicKey, numFinalityProvider)
	for i := 0; i < numFinalityProvider; i++ {
		fpPrivKey, err := btcec.NewPrivateKey()
		require.NoError(t, err)

		finalityProviderKeys[i] = fpPrivKey.PubKey()
	}

	stakingAmount := btcutil.Amount(r.Int63n(1000000000) + 10000)
	stakingTime := uint16(r.Int31n(math.MaxUint16-1) + 1)

	return &TestStakingData{
		StakerKey:            stakerPrivKey.PubKey(),
		FinalityProviderKeys: finalityProviderKeys,
		StakingAmount:        stakingAmount,
		StakingTime:          stakingTime,
	}
}

func GenerateTxFromTestData(t *testing.T, params *TestStakingParams, stakingData *TestStakingData) (*btcstaking.IdentifiableStakingInfo, *btcutil.Tx) {
	stakingInfo, tx, err := btcstaking.BuildV0IdentifiableStakingOutputsAndTx(
		params.MagicBytes,
		stakingData.StakerKey,
		stakingData.FinalityProviderKeys[0],
		params.CovenantKeys,
		params.CovenantQuorum,
		stakingData.StakingTime,
		stakingData.StakingAmount,
		params.Net,
	)
	require.NoError(t, err)

	return stakingInfo, btcutil.NewTx(tx)
}

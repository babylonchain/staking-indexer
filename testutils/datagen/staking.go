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
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/indexerstore"
	"github.com/babylonchain/staking-indexer/types"
)

type TestStakingData struct {
	StakerKey            *btcec.PublicKey
	FinalityProviderKeys []*btcec.PublicKey
	StakingAmount        btcutil.Amount
	StakingTime          uint16
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

func GenerateStakingTxFromTestData(t *testing.T, r *rand.Rand, params *types.Params, stakingData *TestStakingData) (*btcstaking.IdentifiableStakingInfo, *btcutil.Tx) {
	stakingInfo, tx, err := btcstaking.BuildV0IdentifiableStakingOutputsAndTx(
		params.MagicBytes,
		stakingData.StakerKey,
		stakingData.FinalityProviderKeys[0],
		params.CovenantPks,
		params.CovenantQuorum,
		stakingData.StakingTime,
		stakingData.StakingAmount,
		&chaincfg.SigNetParams,
	)
	require.NoError(t, err)

	// an input is needed because btcd serialization does not work well if tx does not have inputs
	txIn := &wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.HashH(bbndatagen.GenRandomByteArray(r, 10)),
			Index: r.Uint32(),
		},
		SignatureScript: bbndatagen.GenRandomByteArray(r, 10),
		Sequence:        r.Uint32(),
	}
	tx.AddTxIn(txIn)

	return stakingInfo, btcutil.NewTx(tx)
}

func GenNStoredStakingTxs(t *testing.T, r *rand.Rand, n int, maxStakingTime uint16) []*indexerstore.StoredStakingTransaction {
	storedTxs := make([]*indexerstore.StoredStakingTransaction, n)

	startingHeight := uint64(r.Int63n(10000) + 1)

	for i := 0; i < n; i++ {
		storedTxs[i] = genStoredStakingTx(t, r, maxStakingTime, startingHeight+uint64(i))
	}

	return storedTxs
}

func GenRandomTx(r *rand.Rand) *wire.MsgTx {
	// structure of the below tx is from https://github.com/btcsuite/btcd/blob/master/wire/msgtx_test.go
	tx := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.HashH(bbndatagen.GenRandomByteArray(r, 10)),
					Index: r.Uint32(),
				},
				SignatureScript: bbndatagen.GenRandomByteArray(r, 10),
				Sequence:        r.Uint32(),
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value:    r.Int63(),
				PkScript: bbndatagen.GenRandomByteArray(r, 80),
			},
		},
		LockTime: 0,
	}

	return tx
}

func genStoredStakingTx(t *testing.T, r *rand.Rand, maxStakingTime uint16, inclusionHeight uint64) *indexerstore.StoredStakingTransaction {
	btcTx := GenRandomTx(r)
	outputIdx := r.Uint32()
	stakingTime := r.Int31n(int32(maxStakingTime)) + 1

	numPubKeys := r.Intn(3) + 1
	stakerPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	fpBtcPks := make([]*btcec.PublicKey, numPubKeys)
	for i := 0; i < numPubKeys; i++ {
		fpPirvKey, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		fpBtcPks[i] = fpPirvKey.PubKey()
	}

	return &indexerstore.StoredStakingTransaction{
		Tx:                  btcTx,
		StakingOutputIdx:    outputIdx,
		StakingTime:         uint32(stakingTime),
		FinalityProviderPks: fpBtcPks,
		StakerPk:            stakerPrivKey.PubKey(),
		InclusionHeight:     inclusionHeight,
	}
}

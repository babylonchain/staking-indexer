package datagen

import (
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
	StakerKey           *btcec.PublicKey
	FinalityProviderKey *btcec.PublicKey
	StakingAmount       btcutil.Amount
	StakingTime         uint16
}

func GenerateTestStakingData(
	t *testing.T,
	r *rand.Rand,
	params *types.GlobalParams,
) *TestStakingData {
	stakerPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	fpPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	stakingAmount := btcutil.Amount(r.Int63n(int64(params.MaxStakingAmount-params.MinStakingAmount)) + int64(params.MinStakingAmount) + 1)
	stakingTime := uint16(r.Intn(int(params.MaxStakingTime-params.MinStakingTime)) + int(params.MinStakingTime))

	return &TestStakingData{
		StakerKey:           stakerPrivKey.PubKey(),
		FinalityProviderKey: fpPrivKey.PubKey(),
		StakingAmount:       stakingAmount,
		StakingTime:         stakingTime,
	}
}

func GenerateStakingTxFromTestData(t *testing.T, r *rand.Rand, params *types.GlobalParams, stakingData *TestStakingData) (*btcstaking.IdentifiableStakingInfo, *btcutil.Tx) {
	stakingInfo, tx, err := btcstaking.BuildV0IdentifiableStakingOutputsAndTx(
		params.Tag,
		stakingData.StakerKey,
		stakingData.FinalityProviderKey,
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

func GenerateUnbondingTxFromStaking(t *testing.T, params *types.GlobalParams, stakingData *TestStakingData, stakingTxHash *chainhash.Hash, stakingOutputIdx uint32) *btcutil.Tx {
	unbondingInfo, err := btcstaking.BuildUnbondingInfo(
		stakingData.StakerKey,
		[]*btcec.PublicKey{stakingData.FinalityProviderKey},
		params.CovenantPks,
		params.CovenantQuorum,
		params.UnbondingTime,
		stakingData.StakingAmount-params.UnbondingFee,
		&chaincfg.SigNetParams,
	)
	require.NoError(t, err)

	unbondingTx := wire.NewMsgTx(2)
	unbondingTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(stakingTxHash, stakingOutputIdx), nil, nil))
	unbondingTx.AddTxOut(unbondingInfo.UnbondingOutput)

	return btcutil.NewTx(unbondingTx)
}

func GenNStoredStakingTxs(t *testing.T, r *rand.Rand, n int, maxStakingTime uint16) []*indexerstore.StoredStakingTransaction {
	storedTxs := make([]*indexerstore.StoredStakingTransaction, n)

	startingHeight := uint64(r.Int63n(10000) + 1)

	for i := 0; i < n; i++ {
		storedTxs[i] = genStoredStakingTx(t, r, maxStakingTime, startingHeight+uint64(i))
	}

	return storedTxs
}

func GenStoredUnbondingTxs(r *rand.Rand, stakingTxs []*indexerstore.StoredStakingTransaction) []*indexerstore.StoredUnbondingTransaction {
	n := len(stakingTxs)
	storedTxs := make([]*indexerstore.StoredUnbondingTransaction, n)

	for i := 0; i < n; i++ {
		stakingHash := stakingTxs[i].Tx.TxHash()
		storedTxs[i] = genStoredUnbondingTx(r, &stakingHash)
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

	stakerPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	fpPirvKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	// Generate a random BTC value range from 0.1 to 1000
	randomBTC := r.Float64()*(999.9-0.1) + 0.1
	stakingValue := btcutil.Amount(randomBTC * btcutil.SatoshiPerBitcoin)

	return &indexerstore.StoredStakingTransaction{
		Tx:                 btcTx,
		StakingOutputIdx:   outputIdx,
		StakingTime:        uint32(stakingTime),
		FinalityProviderPk: fpPirvKey.PubKey(),
		StakerPk:           stakerPrivKey.PubKey(),
		InclusionHeight:    inclusionHeight,
		StakingValue:       uint64(stakingValue),
		IsOverflow:         false,
	}
}

func genStoredUnbondingTx(r *rand.Rand, stakingTxHash *chainhash.Hash) *indexerstore.StoredUnbondingTransaction {
	btcTx := GenRandomTx(r)

	return &indexerstore.StoredUnbondingTransaction{
		Tx:            btcTx,
		StakingTxHash: stakingTxHash,
	}
}

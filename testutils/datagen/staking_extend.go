package datagen

import (
	"math/rand"
	"testing"

	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	"github.com/babylonchain/networks/parameters/parser"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"

	// "github.com/babylonchain/staking-indexer/indexerstore"

	"github.com/scalarorg/btcvault"
	"github.com/scalarorg/staking-indexer/indexerstore"
)

type TestVaultData struct {
	StakerKey                   *btcec.PublicKey
	FinalityProviderKey         *btcec.PublicKey
	StakingAmount               btcutil.Amount
	ChainID                     []byte
	ChainIdUserAddress          []byte
	ChainIdSmartContractAddress []byte
	MintingAmount               []byte
}

func GenerateTestVaultData(
	t *testing.T,
	r *rand.Rand,
	params *parser.ParsedVersionedGlobalParams,
) *TestVaultData {
	stakerPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	fpPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	stakingAmount := btcutil.Amount(r.Int63n(int64(params.MaxStakingAmount-params.MinStakingAmount)) + int64(params.MinStakingAmount) + 1)

	chainID := bbndatagen.GenRandomByteArray(r, 8)
	chainIdUserAddress := bbndatagen.GenRandomByteArray(r, 20)
	chainIdSmartContractAddress := bbndatagen.GenRandomByteArray(r, 20)
	mintingAmount := bbndatagen.GenRandomByteArray(r, 32)

	return &TestVaultData{
		StakerKey:                   stakerPrivKey.PubKey(),
		FinalityProviderKey:         fpPrivKey.PubKey(),
		StakingAmount:               stakingAmount,
		ChainID:                     chainID,
		ChainIdUserAddress:          chainIdUserAddress,
		ChainIdSmartContractAddress: chainIdSmartContractAddress,
		MintingAmount:               mintingAmount,
	}
}

func GenerateVaultTxFromTestData(t *testing.T, r *rand.Rand, params *parser.ParsedVersionedGlobalParams, vaultData *TestVaultData) (*btcvault.IdentifiableVaultInfo, *btcutil.Tx) {
	stakingInfo, tx, err := btcvault.BuildV0IdentifiableVaultOutputsAndTx(
		params.Tag,
		vaultData.StakerKey,
		vaultData.FinalityProviderKey,
		params.CovenantPks,
		params.CovenantQuorum,
		vaultData.StakingAmount,
		vaultData.ChainID,
		vaultData.ChainIdUserAddress,
		vaultData.ChainIdSmartContractAddress,
		vaultData.MintingAmount,
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

func GenerateBurningTxFromVault(t *testing.T, params *parser.ParsedVersionedGlobalParams, vaultData *TestVaultData, stakingTxHash *chainhash.Hash, stakingOutputIdx uint32) *btcutil.Tx {
	vaultInfo, err := btcvault.BuildV0IdentifiableVaultOutputs(
		params.Tag,
		vaultData.StakerKey,
		vaultData.FinalityProviderKey,
		params.CovenantPks,
		params.CovenantQuorum,
		vaultData.StakingAmount,
		vaultData.ChainID,
		vaultData.ChainIdUserAddress,
		vaultData.ChainIdSmartContractAddress,
		vaultData.MintingAmount,
		&chaincfg.SigNetParams,
	)
	require.NoError(t, err)

	burningSpendInfo, err := vaultInfo.BurnPathSpendInfo()
	require.NoError(t, err)

	burningInfo, err := btcvault.BuildBurningInfo(
		vaultData.StakerKey,
		[]*btcec.PublicKey{vaultData.FinalityProviderKey},
		params.CovenantPks,
		params.CovenantQuorum,
		vaultData.StakingAmount-params.UnbondingFee,
		&chaincfg.SigNetParams,
	)
	require.NoError(t, err)

	burningTx := wire.NewMsgTx(2)
	witness, err := btcvault.CreateWitness(burningSpendInfo, [][]byte{})
	require.NoError(t, err)
	burningTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(stakingTxHash, stakingOutputIdx), nil, witness))
	burningTx.TxIn[0].Sequence = wire.MaxTxInSequenceNum
	burningTx.AddTxOut(burningInfo.BurningOutput)

	return btcutil.NewTx(burningTx)
}

func GenerateWithdrawalTxFromVault(t *testing.T, r *rand.Rand, params *parser.ParsedVersionedGlobalParams, vaultData *TestVaultData, vaultTxHash *chainhash.Hash, vaultOutputIdx uint32) *btcutil.Tx {
	vaultInfo, err := btcvault.BuildV0IdentifiableVaultOutputs(
		params.Tag,
		vaultData.StakerKey,
		vaultData.FinalityProviderKey,
		params.CovenantPks,
		params.CovenantQuorum,
		vaultData.StakingAmount,
		vaultData.ChainID,
		vaultData.ChainIdUserAddress,
		vaultData.ChainIdSmartContractAddress,
		vaultData.MintingAmount,
		&chaincfg.SigNetParams,
	)
	require.NoError(t, err)

	burnwithoutDAppSpendInfo, err := vaultInfo.BurnWithoutDAppPathSpendInfo()
	require.NoError(t, err)

	burnwithoutDAppTx := wire.NewMsgTx(2)
	witness, err := btcvault.CreateWitness(burnwithoutDAppSpendInfo, [][]byte{})
	require.NoError(t, err)

	burnwithoutDAppTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(vaultTxHash, vaultOutputIdx), nil, witness))
	// add a dump input
	randomOutput := &wire.OutPoint{
		Hash:  chainhash.HashH(bbndatagen.GenRandomByteArray(r, 10)),
		Index: r.Uint32(),
	}
	burnwithoutDAppTx.AddTxIn(wire.NewTxIn(randomOutput, bbndatagen.GenRandomByteArray(r, 10), nil))

	return btcutil.NewTx(burnwithoutDAppTx)
}

func GenerateWithdrawalTxFromBurning(t *testing.T, r *rand.Rand, params *parser.ParsedVersionedGlobalParams, vaultData *TestVaultData, burningTxHash *chainhash.Hash) *btcutil.Tx {
	// build and send withdraw tx from the unbonding tx
	burningInfo, err := btcvault.BuildBurningInfo(
		vaultData.StakerKey,
		[]*btcec.PublicKey{vaultData.FinalityProviderKey},
		params.CovenantPks,
		params.CovenantQuorum,
		vaultData.StakingAmount.MulF64(0.9),
		&chaincfg.SigNetParams,
	)
	require.NoError(t, err)
	burnWithoutDAppSpendInfo, err := burningInfo.BurnWithoutDAppPathSpendInfo()
	require.NoError(t, err)

	burnWithoutDAppTx := wire.NewMsgTx(2)
	witness, err := btcvault.CreateWitness(burnWithoutDAppSpendInfo, [][]byte{})
	require.NoError(t, err)

	burnWithoutDAppTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(burningTxHash, 0), nil, witness))
	// add a dump input
	randomOutput := &wire.OutPoint{
		Hash:  chainhash.HashH(bbndatagen.GenRandomByteArray(r, 10)),
		Index: r.Uint32(),
	}
	burnWithoutDAppTx.AddTxIn(wire.NewTxIn(randomOutput, bbndatagen.GenRandomByteArray(r, 10), nil))

	return btcutil.NewTx(burnWithoutDAppTx)
}

func GenNStoredVaultTxs(t *testing.T, r *rand.Rand, n int, maxStakingTime uint16) []*indexerstore.StoredVaultTransaction {
	storedTxs := make([]*indexerstore.StoredVaultTransaction, n)

	startingHeight := uint64(r.Int63n(10000) + 1)

	for i := 0; i < n; i++ {
		storedTxs[i] = genStoredVaultTx(t, r, startingHeight+uint64(i))
	}

	return storedTxs
}

func GenStoredBurningTxs(r *rand.Rand, stakingTxs []*indexerstore.StoredVaultTransaction) []*indexerstore.StoredBurningTransaction {
	n := len(stakingTxs)
	storedTxs := make([]*indexerstore.StoredBurningTransaction, n)

	for i := 0; i < n; i++ {
		stakingHash := stakingTxs[i].Tx.TxHash()
		storedTxs[i] = genStoredBurningTx(r, &stakingHash)
	}

	return storedTxs
}

func genStoredVaultTx(t *testing.T, r *rand.Rand, inclusionHeight uint64) *indexerstore.StoredVaultTransaction {
	btcTx := GenRandomTx(r)
	outputIdx := r.Uint32()

	stakerPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	dAppPirvKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	// Generate a random BTC value range from 0.1 to 1000
	randomBTC := r.Float64()*(999.9-0.1) + 0.1
	stakingValue := btcutil.Amount(randomBTC * btcutil.SatoshiPerBitcoin)

	//
	chainID := bbndatagen.GenRandomByteArray(r, 8)
	chainIdUserAddress := bbndatagen.GenRandomByteArray(r, 20)
	chainIdSmartContractAddress := bbndatagen.GenRandomByteArray(r, 20)
	mintingAmount := bbndatagen.GenRandomByteArray(r, 32)

	return &indexerstore.StoredVaultTransaction{
		Tx:                          btcTx,
		StakingOutputIdx:            outputIdx,
		DAppPk:                      dAppPirvKey.PubKey(),
		StakerPk:                    stakerPrivKey.PubKey(),
		InclusionHeight:             inclusionHeight,
		StakingValue:                uint64(stakingValue),
		ChainID:                     chainID,
		ChainIdUserAddress:          chainIdUserAddress,
		ChainIdSmartContractAddress: chainIdSmartContractAddress,
		MintingAmount:               mintingAmount,
		IsOverflow:                  false,
	}
}

func genStoredBurningTx(r *rand.Rand, vaultTxhash *chainhash.Hash) *indexerstore.StoredBurningTransaction {
	btcTx := GenRandomTx(r)

	return &indexerstore.StoredBurningTransaction{
		Tx:          btcTx,
		VaultTxHash: vaultTxhash,
	}
}

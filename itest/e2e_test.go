//go:build e2e
// +build e2e

package e2etest

import (
	"math/rand"
	"testing"
	"time"

	"github.com/babylonchain/babylon/btcstaking"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/params"
	"github.com/babylonchain/staking-indexer/testutils/datagen"
	"github.com/babylonchain/staking-indexer/types"
)

func TestBTCScanner(t *testing.T) {
	n := 100
	tm := StartManagerWithNBlocks(t, n)
	defer tm.Stop()

	count, err := tm.BitcoindHandler.GetBlockCount()
	require.NoError(t, err)
	require.Equal(t, n, count)

	require.Eventually(t, func() bool {
		confirmedTip := tm.BS.LastConfirmedHeight()
		return confirmedTip == uint64(n-int(tm.Config.BTCScannerConfig.ConfirmationDepth))
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

func TestStakingIndexer(t *testing.T) {
	// ensure we have UTXOs
	n := 110
	tm := StartManagerWithNBlocks(t, n)
	defer tm.Stop()
	k := tm.Config.BTCScannerConfig.ConfirmationDepth

	// generate valid staking tx data
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	testStakingData := datagen.GenerateTestStakingData(t, r)
	sysParams, err := params.NewLocalParamsRetriever().GetParams()
	require.NoError(t, err)
	stakingInfo, err := btcstaking.BuildV0IdentifiableStakingOutputs(
		sysParams.MagicBytes,
		tm.WalletPrivKey.PubKey(),
		testStakingData.FinalityProviderKey,
		sysParams.CovenantPks,
		sysParams.CovenantQuorum,
		testStakingData.StakingTime,
		testStakingData.StakingAmount,
		regtestParams,
	)
	require.NoError(t, err)

	// send the staking tx and mine blocks
	err = tm.StakerWallet.UnlockWallet(20)
	require.NoError(t, err)
	stakingTx, err := tm.StakerWallet.CreateAndSignTx(
		[]*wire.TxOut{stakingInfo.OpReturnOutput, stakingInfo.StakingOutput},
		1000,
		tm.MinerAddr,
	)
	require.NoError(t, err)
	m := int(tm.Config.BTCScannerConfig.ConfirmationDepth + 1)
	tm.SendTxWithNConfirmations(t, stakingTx, m)

	// wait until the indexer parses the staking tx
	tm.WaitForConfirmedTipHeight(t, k)

	// check that the staking tx is already stored
	stakingTxHash := stakingTx.TxHash()
	storedStakingTx, err := tm.Si.GetStakingTxByHash(&stakingTxHash)
	require.NoError(t, err)
	require.Equal(t, stakingTx.TxHash().String(), storedStakingTx.Tx.TxHash().String())

	// check the staking event is received by the queue
	tm.CheckNextStakingEvent(t, stakingTx.TxHash().String())

	// build and send unbonding tx from the previous staking tx
	unbondingSpendInfo, err := stakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)
	unbondingTx := buildUnbondingTx(
		t,
		sysParams,
		tm.WalletPrivKey,
		testStakingData.FinalityProviderKey,
		testStakingData.StakingAmount,
		&stakingTxHash,
		storedStakingTx.StakingOutputIdx,
		unbondingSpendInfo,
		stakingTx,
		[]*btcec.PrivateKey{params.CovenantPrivKey},
	)
	tm.SendTxWithNConfirmations(t, unbondingTx, m)

	// wait until the indexer identifies the unbonding tx
	tm.WaitForConfirmedTipHeight(t, k)

	// check that the unbonding tx is already stored
	unbondingTxHash := unbondingTx.TxHash()
	storedUnbondingTx, err := tm.Si.GetUnbondingTxByHash(&unbondingTxHash)
	require.NoError(t, err)
	require.Equal(t, unbondingTx.TxHash().String(), storedUnbondingTx.Tx.TxHash().String())
	require.Equal(t, stakingTxHash.String(), storedUnbondingTx.StakingTxHash.String())

	// check the unbonding event is received by the queue
	tm.CheckNextUnbondingEvent(t, stakingTx.TxHash().String())
}

func buildUnbondingTx(
	t *testing.T,
	params *types.Params,
	stakerPrivKey *btcec.PrivateKey,
	fpKey *btcec.PublicKey,
	stakingAmount btcutil.Amount,
	stakingTxHash *chainhash.Hash,
	stakingOutputIdx uint32,
	unbondingSpendInfo *btcstaking.SpendInfo,
	stakingTx *wire.MsgTx,
	covPrivKeys []*btcec.PrivateKey,
) *wire.MsgTx {
	unbondingInfo, err := btcstaking.BuildUnbondingInfo(
		stakerPrivKey.PubKey(),
		[]*btcec.PublicKey{fpKey},
		params.CovenantPks,
		params.CovenantQuorum,
		params.UnbondingTime,
		stakingAmount.MulF64(0.9),
		regtestParams,
	)
	require.NoError(t, err)

	unbondingTx := wire.NewMsgTx(2)
	unbondingTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(stakingTxHash, stakingOutputIdx), nil, nil))
	unbondingTx.AddTxOut(unbondingInfo.UnbondingOutput)

	// generate covenant unbonding sigs
	unbondingCovSigs := make([]*schnorr.Signature, len(covPrivKeys))
	for i, privKey := range covPrivKeys {
		sig, err := btcstaking.SignTxWithOneScriptSpendInputStrict(
			unbondingTx,
			stakingTx,
			stakingOutputIdx,
			unbondingSpendInfo.GetPkScriptPath(),
			privKey,
		)
		require.NoError(t, err)

		unbondingCovSigs[i] = sig
	}

	stakerUnbondingSig, err := btcstaking.SignTxWithOneScriptSpendInputFromScript(
		unbondingTx,
		stakingTx.TxOut[stakingOutputIdx],
		stakerPrivKey,
		unbondingSpendInfo.RevealedLeaf.Script,
	)

	witness, err := unbondingSpendInfo.CreateUnbondingPathWitness(unbondingCovSigs, stakerUnbondingSig)
	require.NoError(t, err)
	unbondingTx.TxIn[0].Witness = witness

	return unbondingTx
}

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
	"github.com/btcsuite/btcd/txscript"
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

// TestStakingLifeCycle covers the following life cycle
// 1. the staking tx is sent to BTC
// 2. the staking tx is parsed by the indexer
// 3. wait until the staking tx expires
// 4. the subsequent withdraw tx is sent to BTC
// 5. the withdraw tx is identified by the indexer
func TestStakingLifeCycle(t *testing.T) {
	// ensure we have UTXOs
	n := 110
	tm := StartManagerWithNBlocks(t, n)
	defer tm.Stop()
	k := tm.Config.BTCScannerConfig.ConfirmationDepth

	// generate valid staking tx data
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	testStakingData := datagen.GenerateTestStakingData(t, r)
	testStakingData.StakingTime = 120
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
	tm.SendTxWithNConfirmations(t, stakingTx, int(k+1))

	// wait for the indexer to process confirmed blocks
	tm.WaitForNConfirmations(t, int(k))

	// check that the staking tx is already stored
	stakingTxHash := stakingTx.TxHash()
	storedStakingTx, err := tm.Si.GetStakingTxByHash(&stakingTxHash)
	require.NoError(t, err)
	require.Equal(t, stakingTx.TxHash().String(), storedStakingTx.Tx.TxHash().String())

	// check the staking event is received by the queue
	tm.CheckNextStakingEvent(t, stakingTx.TxHash().String())

	// wait for the staking tx expires
	if uint64(testStakingData.StakingTime) > k {
		tm.BitcoindHandler.GenerateBlocks(int(uint64(testStakingData.StakingTime) - k))
	}

	// build and send withdraw tx
	withdrawSpendInfo, err := stakingInfo.TimeLockPathSpendInfo()
	require.NoError(t, err)

	require.NoError(t, err)
	withdrawTx := buildStakingWithdrawTx(
		t,
		tm.WalletPrivKey,
		stakingTx.TxOut[storedStakingTx.StakingOutputIdx],
		stakingTx.TxHash(),
		storedStakingTx.StakingOutputIdx,
		withdrawSpendInfo,
		testStakingData.StakingTime,
		testStakingData.StakingAmount,
	)
	tm.SendTxWithNConfirmations(t, withdrawTx, int(k+1))

	// wait until the indexer identifies the withdraw tx
	tm.WaitForNConfirmations(t, int(k))

	// check the withdraw event is received
	tm.CheckNextWithdrawEvent(t, stakingTx.TxHash().String())
}

// TestStakingIndexer_StakingUnbondingLifeCycle covers the following life cycle
// 1. the staking tx is sent to BTC
// 2. the staking tx is parsed by the indexer
// 3. the subsequent unbonding tx is sent to BTC
// 4. the unbonding tx is identified by the indexer
// 5. the subsequent withdraw tx is sent to BTC
// 6. the withdraw tx is identified by the indexer
func TestStakingIndexer_StakingUnbondingLifeCycle(t *testing.T) {
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
	tm.SendTxWithNConfirmations(t, stakingTx, int(k+1))

	// wait for the indexer to process confirmed blocks
	tm.WaitForNConfirmations(t, int(k))

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
	tm.SendTxWithNConfirmations(t, unbondingTx, int(k+1))

	// wait for the indexer to process confirmed blocks
	tm.WaitForNConfirmations(t, int(k))

	// check the unbonding tx is already stored
	unbondingTxHash := unbondingTx.TxHash()
	storedUnbondingTx, err := tm.Si.GetUnbondingTxByHash(&unbondingTxHash)
	require.NoError(t, err)
	require.Equal(t, unbondingTx.TxHash().String(), storedUnbondingTx.Tx.TxHash().String())
	require.Equal(t, stakingTxHash.String(), storedUnbondingTx.StakingTxHash.String())

	// check the unbonding event is received
	tm.CheckNextUnbondingEvent(t, stakingTx.TxHash().String())

	// wait for the unbonding tx expires
	if uint64(sysParams.UnbondingTime) > k {
		tm.BitcoindHandler.GenerateBlocks(int(uint64(sysParams.UnbondingTime) - k))
	}

	// build and send withdraw tx

	// wait until the indexer identifies the withdraw tx

	// check the withdraw event is received
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

func buildStakingWithdrawTx(
	t *testing.T,
	stakerPrivKey *btcec.PrivateKey,
	stakingOutput *wire.TxOut,
	stakingHash chainhash.Hash,
	stakingOutputIndex uint32,
	stakingSpendInfo *btcstaking.SpendInfo,
	lockTime uint16,
	lockedAmount btcutil.Amount,
) *wire.MsgTx {

	destAddress, err := btcutil.NewAddressPubKey(stakerPrivKey.PubKey().SerializeCompressed(), regtestParams)
	require.NoError(t, err)
	destAddressScript, err := txscript.PayToAddrScript(destAddress)
	require.NoError(t, err)

	// to spend output with relative timelock transaction need to be version two or higher
	withdrawTx := wire.NewMsgTx(2)
	withdrawTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&stakingHash, stakingOutputIndex), nil, nil))
	withdrawTx.AddTxOut(wire.NewTxOut(int64(lockedAmount.MulF64(0.5)), destAddressScript))

	// we need to set sequence number before signing, as signing commits to sequence
	// number
	withdrawTx.TxIn[0].Sequence = uint32(lockTime)

	sig, err := btcstaking.SignTxWithOneScriptSpendInputFromTapLeaf(
		withdrawTx,
		stakingOutput,
		stakerPrivKey,
		stakingSpendInfo.RevealedLeaf,
	)

	require.NoError(t, err)

	witness, err := stakingSpendInfo.CreateTimeLockPathWitness(sig)

	require.NoError(t, err)

	withdrawTx.TxIn[0].Witness = witness

	return withdrawTx
}

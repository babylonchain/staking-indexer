//go:build e2e
// +build e2e

package e2etest

import (
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"github.com/babylonchain/babylon/btcstaking"
	bbndatagen "github.com/babylonchain/babylon/testutil/datagen"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/config"
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

	tm.WaitForNConfirmations(t, int(tm.Config.BTCScannerConfig.ConfirmationDepth))

	_ = tm.BitcoindHandler.GenerateBlocks(10)

	tm.WaitForNConfirmations(t, int(tm.Config.BTCScannerConfig.ConfirmationDepth))
}

func TestQueueConsumer(t *testing.T) {
	// create event consumer
	queueCfg := config.DefaultQueueConfig()
	queueConsumer, err := setupTestQueueConsumer(t, queueCfg)
	require.NoError(t, err)
	stakingChan, err := queueConsumer.StakingQueue.ReceiveMessages()
	require.NoError(t, err)

	defer queueConsumer.Stop()

	n := 1
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	stakingEventList := make([]*types.ActiveStakingEvent, 0)
	for i := 0; i < n; i++ {
		stakingEvent := &types.ActiveStakingEvent{
			EventType:        types.ActiveStakingEventType,
			StakingTxHashHex: hex.EncodeToString(bbndatagen.GenRandomByteArray(r, 10)),
		}
		err = queueConsumer.PushStakingEvent(stakingEvent)
		require.NoError(t, err)
		stakingEventList = append(stakingEventList, stakingEvent)
	}

	for i := 0; i < n; i++ {
		stakingEventBytes := <-stakingChan
		var receivedStakingEvent types.ActiveStakingEvent
		err = json.Unmarshal([]byte(stakingEventBytes.Body), &receivedStakingEvent)
		require.NoError(t, err)
		require.Equal(t, stakingEventList[i].StakingTxHashHex, receivedStakingEvent.StakingTxHashHex)
		err = queueConsumer.StakingQueue.DeleteMessage(stakingEventBytes.Receipt)
		require.NoError(t, err)
	}
}

// TestStakingLifeCycle covers the following life cycle
// 1. the staking tx is sent to BTC
// 2. the staking tx is parsed by the indexer
// 3. wait until the staking tx expires
// 4. the subsequent withdraw tx is sent to BTC
// 5. the withdraw tx is identified by the indexer and consumed by the queue
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
	stakingTxHash := stakingTx.TxHash()
	tm.SendTxWithNConfirmations(t, stakingTx, int(k+1))

	// check that the staking tx is already stored
	tm.WaitForStakingTxStored(t, stakingTxHash)

	// check the staking event is received by the queue
	tm.CheckNextStakingEvent(t, stakingTxHash)

	// wait for the staking tx expires
	if uint64(testStakingData.StakingTime) > k {
		tm.BitcoindHandler.GenerateBlocks(int(uint64(testStakingData.StakingTime) - k))
	}

	// build and send withdraw tx and mine blocks
	withdrawSpendInfo, err := stakingInfo.TimeLockPathSpendInfo()
	require.NoError(t, err)

	storedStakingTx, err := tm.Si.GetStakingTxByHash(&stakingTxHash)
	require.NoError(t, err)
	withdrawTx := buildWithdrawTx(
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

	// check the withdraw event is received
	tm.CheckNextWithdrawEvent(t, stakingTx.TxHash())
}

// TestStakingUnbondingLifeCycle covers the following life cycle
// 1. the staking tx is sent to BTC
// 2. the staking tx is parsed by the indexer
// 3. the subsequent unbonding tx is sent to BTC
// 4. the unbonding tx is identified by the indexer
// 5. the subsequent withdraw tx is sent to BTC
// 6. the withdraw tx is identified by the indexer
func TestStakingUnbondingLifeCycle(t *testing.T) {
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
	stakingTxHash := stakingTx.TxHash()
	tm.SendTxWithNConfirmations(t, stakingTx, int(k+1))

	// check that the staking tx is already stored
	tm.WaitForStakingTxStored(t, stakingTxHash)

	// check the staking event is received by the queue
	tm.CheckNextStakingEvent(t, stakingTxHash)

	// build and send unbonding tx from the previous staking tx
	unbondingSpendInfo, err := stakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)
	storedStakingTx, err := tm.Si.GetStakingTxByHash(&stakingTxHash)
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

	// check the unbonding tx is already stored
	tm.WaitForUnbondingTxStored(t, unbondingTx.TxHash())

	// check the unbonding event is received
	tm.CheckNextUnbondingEvent(t, unbondingTx.TxHash())

	// wait for the unbonding tx expires
	if uint64(sysParams.UnbondingTime) > k {
		tm.BitcoindHandler.GenerateBlocks(int(uint64(sysParams.UnbondingTime) - k))
	}

	// build and send withdraw tx from the unbonding tx
	unbondingInfo, err := btcstaking.BuildUnbondingInfo(
		tm.WalletPrivKey.PubKey(),
		[]*btcec.PublicKey{testStakingData.FinalityProviderKey},
		sysParams.CovenantPks,
		sysParams.CovenantQuorum,
		sysParams.UnbondingTime,
		testStakingData.StakingAmount.MulF64(0.9),
		regtestParams,
	)
	require.NoError(t, err)
	withdrawSpendInfo, err := unbondingInfo.TimeLockPathSpendInfo()
	require.NoError(t, err)
	withdrawTx := buildWithdrawTx(
		t,
		tm.WalletPrivKey,
		// unbonding tx only has one output
		unbondingTx.TxOut[0],
		unbondingTx.TxHash(),
		0,
		withdrawSpendInfo,
		sysParams.UnbondingTime,
		testStakingData.StakingAmount,
	)
	tm.SendTxWithNConfirmations(t, withdrawTx, int(k+1))

	// wait until the indexer identifies the withdraw tx
	tm.WaitForNConfirmations(t, int(k))

	// check the withdraw event is consumed
	tm.CheckNextWithdrawEvent(t, stakingTx.TxHash())
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

func buildWithdrawTx(
	t *testing.T,
	stakerPrivKey *btcec.PrivateKey,
	fundTxOutput *wire.TxOut,
	fundTxHash chainhash.Hash,
	fundTxOutputIndex uint32,
	fundTxSpendInfo *btcstaking.SpendInfo,
	lockTime uint16,
	lockedAmount btcutil.Amount,
) *wire.MsgTx {

	destAddress, err := btcutil.NewAddressPubKey(stakerPrivKey.PubKey().SerializeCompressed(), regtestParams)
	require.NoError(t, err)
	destAddressScript, err := txscript.PayToAddrScript(destAddress)
	require.NoError(t, err)

	// to spend output with relative timelock transaction need to be version two or higher
	withdrawTx := wire.NewMsgTx(2)
	withdrawTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&fundTxHash, fundTxOutputIndex), nil, nil))
	withdrawTx.AddTxOut(wire.NewTxOut(int64(lockedAmount.MulF64(0.5)), destAddressScript))

	// we need to set sequence number before signing, as signing commits to sequence
	// number
	withdrawTx.TxIn[0].Sequence = uint32(lockTime)

	sig, err := btcstaking.SignTxWithOneScriptSpendInputFromTapLeaf(
		withdrawTx,
		fundTxOutput,
		stakerPrivKey,
		fundTxSpendInfo.RevealedLeaf,
	)

	require.NoError(t, err)

	witness, err := fundTxSpendInfo.CreateTimeLockPathWitness(sig)

	require.NoError(t, err)

	withdrawTx.TxIn[0].Witness = witness

	return withdrawTx
}

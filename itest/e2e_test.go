package e2etest

import (
	"math/rand"
	"testing"
	"time"

	"github.com/babylonchain/babylon/btcstaking"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/params"
	"github.com/babylonchain/staking-indexer/testutils/datagen"
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
	tm.SendStakingTxWithNConfirmations(t, stakingTx, m)

	// wait until the indexer parses the staking tx
	require.Eventually(t, func() bool {
		confirmedTip := tm.BS.LastConfirmedHeight()
		return confirmedTip == uint64(m+n-int(tm.Config.BTCScannerConfig.ConfirmationDepth))
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// check that the staking tx is already stored
	stakingTxHash := stakingTx.TxHash()
	storedStakingTx, err := tm.Si.GetStakingTxByHash(&stakingTxHash)
	require.NoError(t, err)
	require.Equal(t, stakingTx.TxHash().String(), storedStakingTx.Tx.TxHash().String())
}

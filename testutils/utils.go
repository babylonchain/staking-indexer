package testutils

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"testing"

	"github.com/babylonchain/babylon/btcstaking"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet/txauthor"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/types"
)

type Utxo struct {
	Amount       btcutil.Amount
	OutPoint     wire.OutPoint
	PkScript     []byte
	RedeemScript []byte
	Address      string
}

type byAmount []Utxo

func (s byAmount) Len() int           { return len(s) }
func (s byAmount) Less(i, j int) bool { return s[i].Amount < s[j].Amount }
func (s byAmount) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func PubKeysEqual(pk1, pk2 *btcec.PublicKey) bool {
	return bytes.Equal(schnorr.SerializePubKey(pk1), schnorr.SerializePubKey(pk2))
}

func MakeTestBackend(t *testing.T) kvdb.Backend {
	// First, create a temporary directory to be used for the duration of
	// this test.
	tempDirName := t.TempDir()

	cfg := config.DefaultDBConfig()

	cfg.DBPath = tempDirName

	backend, err := cfg.GetDbBackend()
	require.NoError(t, err)

	t.Cleanup(func() {
		backend.Close()
	})

	return backend
}

func PubKeysSliceEqual(pk1, pk2 []*btcec.PublicKey) bool {
	if len(pk1) != len(pk2) {
		return false
	}

	for i := 0; i < len(pk1); i++ {
		if !PubKeysEqual(pk1[i], pk2[i]) {
			return false
		}
	}

	return true
}

// Adapted from
// https://github.com/babylonchain/btc-staker/blob/eb72d300bc263706a72e459ea975abfa0467f2dc/walletcontroller/client.go#L148
func CreateTxFromOutputsAndSign(
	btcClient *rpcclient.Client,
	outputs []*wire.TxOut,
	feeRatePerKb btcutil.Amount,
	changeAddres btcutil.Address,
) (*wire.MsgTx, error) {
	utxoResults, err := btcClient.ListUnspent()

	if err != nil {
		return nil, err
	}

	utxos, err := resultsToUtxos(utxoResults, true)

	if err != nil {
		return nil, err
	}

	// sort utxos by amount from highest to lowest, this is effectively strategy of using
	// largest inputs first
	sort.Sort(sort.Reverse(byAmount(utxos)))

	changeScript, err := txscript.PayToAddrScript(changeAddres)

	if err != nil {
		return nil, err
	}

	tx, err := buildTxFromOutputs(utxos, outputs, feeRatePerKb, changeScript)

	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	fundedTx, signed, err := btcClient.SignRawTransactionWithWallet(tx)

	if err != nil {
		return nil, err
	}

	if !signed {
		// TODO: Investigate this case a bit more thoroughly, to check if we can recover
		// somehow
		return nil, fmt.Errorf("not all transactions inputs could be signed")
	}

	return fundedTx, nil
}

func resultsToUtxos(results []btcjson.ListUnspentResult, onlySpendable bool) ([]Utxo, error) {
	var utxos []Utxo
	for _, result := range results {
		if onlySpendable && !result.Spendable {
			// skip unspendable outputs
			continue
		}

		amount, err := btcutil.NewAmount(result.Amount)

		if err != nil {
			return nil, err
		}

		txHash, err := chainhash.NewHashFromStr(result.TxID)

		if err != nil {
			return nil, err
		}

		outpoint := wire.NewOutPoint(txHash, result.Vout)

		script, err := hex.DecodeString(result.ScriptPubKey)

		if err != nil {
			return nil, err
		}

		redeemScript, err := hex.DecodeString(result.RedeemScript)

		if err != nil {
			return nil, err
		}

		utxo := Utxo{
			Amount:       amount,
			OutPoint:     *outpoint,
			PkScript:     script,
			RedeemScript: redeemScript,
			Address:      result.Address,
		}
		utxos = append(utxos, utxo)
	}
	return utxos, nil
}

func buildTxFromOutputs(
	utxos []Utxo,
	outputs []*wire.TxOut,
	feeRatePerKb btcutil.Amount,
	changeScript []byte) (*wire.MsgTx, error) {

	if len(utxos) == 0 {
		return nil, fmt.Errorf("there must be at least 1 usable UTXO to build transaction")
	}

	if len(outputs) == 0 {
		return nil, fmt.Errorf("there must be at least 1 output in transaction")
	}

	ch := txauthor.ChangeSource{
		NewScript: func() ([]byte, error) {
			return changeScript, nil
		},
		ScriptSize: len(changeScript),
	}

	inputSource := makeInputSource(utxos)

	authoredTx, err := txauthor.NewUnsignedTransaction(
		outputs,
		feeRatePerKb,
		inputSource,
		&ch,
	)

	if err != nil {
		return nil, err
	}

	return authoredTx.Tx, nil
}

func makeInputSource(utxos []Utxo) txauthor.InputSource {
	currentTotal := btcutil.Amount(0)
	currentInputs := make([]*wire.TxIn, 0, len(utxos))
	currentScripts := make([][]byte, 0, len(utxos))
	currentInputValues := make([]btcutil.Amount, 0, len(utxos))

	return func(target btcutil.Amount) (btcutil.Amount, []*wire.TxIn,
		[]btcutil.Amount, [][]byte, error) {

		for currentTotal < target && len(utxos) != 0 {
			nextCredit := &utxos[0]
			utxos = utxos[1:]
			nextInput := wire.NewTxIn(&nextCredit.OutPoint, nil, nil)
			currentTotal += nextCredit.Amount
			currentInputs = append(currentInputs, nextInput)
			currentScripts = append(currentScripts, nextCredit.PkScript)
			currentInputValues = append(currentInputValues, nextCredit.Amount)
		}
		return currentTotal, currentInputs, currentInputValues, currentScripts, nil
	}
}

func BuildUnbondingTx(
	t *testing.T,
	params *types.GlobalParams,
	stakerPrivKey *btcec.PrivateKey,
	fpKey *btcec.PublicKey,
	stakingAmount btcutil.Amount,
	stakingTxHash *chainhash.Hash,
	stakingOutputIdx uint32,
	unbondingSpendInfo *btcstaking.SpendInfo,
	stakingTx *wire.MsgTx,
	covPrivKeys []*btcec.PrivateKey,
	btcParams *chaincfg.Params,
) *wire.MsgTx {
	expectedOutputValue := stakingAmount - params.UnbondingFee
	unbondingInfo, err := btcstaking.BuildUnbondingInfo(
		stakerPrivKey.PubKey(),
		[]*btcec.PublicKey{fpKey},
		params.CovenantPks,
		params.CovenantQuorum,
		params.UnbondingTime,
		expectedOutputValue,
		btcParams,
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
	require.NoError(t, err)

	witness, err := unbondingSpendInfo.CreateUnbondingPathWitness(unbondingCovSigs, stakerUnbondingSig)
	require.NoError(t, err)
	unbondingTx.TxIn[0].Witness = witness

	return unbondingTx
}

func BuildWithdrawTx(
	t *testing.T,
	stakerPrivKey *btcec.PrivateKey,
	fundTxOutput *wire.TxOut,
	fundTxHash chainhash.Hash,
	fundTxOutputIndex uint32,
	fundTxSpendInfo *btcstaking.SpendInfo,
	lockTime uint16,
	lockedAmount btcutil.Amount,
	btcParams *chaincfg.Params,
) *wire.MsgTx {

	destAddress, err := btcutil.NewAddressPubKey(stakerPrivKey.PubKey().SerializeCompressed(), btcParams)

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

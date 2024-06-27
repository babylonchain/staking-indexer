package testutils

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"testing"

	"github.com/babylonchain/babylon/btcstaking"
	"github.com/babylonchain/networks/parameters/parser"
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
)

type Utxo struct {
	Amount       btcutil.Amount
	OutPoint     wire.OutPoint
	PkScript     []byte
	RedeemScript []byte
	Address      string
}

type FundingInfo struct {
	FundTxOutput      *wire.TxOut
	FundTxHash        chainhash.Hash
	FundTxOutputIndex uint32
	FundTxSpendInfo   *btcstaking.SpendInfo
	LockTime          uint16
	Value             btcutil.Amount
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
	params *parser.ParsedVersionedGlobalParams,
	stakerPrivKey *btcec.PrivateKey,
	fpKey *btcec.PublicKey,
	stakingAmount btcutil.Amount,
	stakingTxHash *chainhash.Hash,
	stakingOutputIdx uint32,
	stakingInfo *btcstaking.IdentifiableStakingInfo,
	stakingTx *wire.MsgTx,
	covPrivKeys []*btcec.PrivateKey,
	btcParams *chaincfg.Params,
) (*wire.MsgTx, *btcstaking.UnbondingInfo) {
	unbondingSpendInfo, err := stakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)
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
	unbondingTx.TxIn[0].Sequence = wire.MaxTxInSequenceNum
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

	return unbondingTx, unbondingInfo
}

func BuildWithdrawTx(
	t *testing.T,
	stakerPrivKey *btcec.PrivateKey,
	fundings []*FundingInfo,
	btcParams *chaincfg.Params,
) *wire.MsgTx {
	destAddress, err := btcutil.NewAddressPubKey(stakerPrivKey.PubKey().SerializeCompressed(), btcParams)

	require.NoError(t, err)
	destAddressScript, err := txscript.PayToAddrScript(destAddress)
	require.NoError(t, err)

	// calculate total value
	var totalValue btcutil.Amount
	for _, f := range fundings {
		totalValue += f.Value
	}

	// to spend output with relative timelock transaction need to be version two or higher
	withdrawTx := wire.NewMsgTx(2)
	withdrawTx.AddTxOut(wire.NewTxOut(int64(totalValue.MulF64(0.5)), destAddressScript))

	fundingOutputs := make([]*wire.TxOut, 0)
	privKeys := make([]*btcec.PrivateKey, 0)
	revealedLeaves := make([]txscript.TapLeaf, 0)
	for _, fundingInfo := range fundings {
		txInput := wire.NewTxIn(wire.NewOutPoint(&fundingInfo.FundTxHash, fundingInfo.FundTxOutputIndex), nil, nil)
		// we need to set sequence number before signing, as signing commits to sequence
		// number
		txInput.Sequence = uint32(fundingInfo.LockTime)
		withdrawTx.AddTxIn(txInput)

		fundingOutputs = append(fundingOutputs, fundingInfo.FundTxOutput)
		// for testing purpose, all the txs are using the same private key
		privKeys = append(privKeys, stakerPrivKey)
		revealedLeaves = append(revealedLeaves, fundingInfo.FundTxSpendInfo.RevealedLeaf)
	}

	sigs, err := SignTxWithMultipleScriptSpendInputFromTapLeaf(
		withdrawTx,
		fundingOutputs,
		privKeys,
		revealedLeaves,
	)
	require.NoError(t, err)

	for i, s := range sigs {
		witness, err := fundings[i].FundTxSpendInfo.CreateTimeLockPathWitness(s)
		require.NoError(t, err)
		withdrawTx.TxIn[i].Witness = witness
	}

	return withdrawTx
}

func signTxWithOneScriptSpendInputFromTapLeafInternalWithIdx(
	txToSign *wire.MsgTx,
	fundingOutput *wire.TxOut,
	idx int,
	fetcher txscript.PrevOutputFetcher,
	privKey *btcec.PrivateKey,
	tapLeaf txscript.TapLeaf) (*schnorr.Signature, error) {

	sigHashes := txscript.NewTxSigHashes(txToSign, fetcher)

	sig, err := txscript.RawTxInTapscriptSignature(
		txToSign, sigHashes, idx, fundingOutput.Value,
		fundingOutput.PkScript, tapLeaf, txscript.SigHashDefault,
		privKey,
	)
	if err != nil {
		return nil, err
	}

	parsedSig, err := schnorr.ParseSignature(sig)
	if err != nil {
		return nil, err
	}

	return parsedSig, nil
}

func SignTxWithMultipleScriptSpendInputFromTapLeaf(
	txToSign *wire.MsgTx,
	fundingOutputs []*wire.TxOut,
	privKeys []*btcec.PrivateKey,
	tapLeafs []txscript.TapLeaf,
) ([]*schnorr.Signature, error) {
	if txToSign == nil {
		return nil, fmt.Errorf("tx to sign must not be nil")
	}

	if len(fundingOutputs) == 0 {
		return nil, fmt.Errorf("funding output must not be nil")
	}

	if len(privKeys) == 0 {
		return nil, fmt.Errorf("private key must not be nil")
	}

	var signatures []*schnorr.Signature

	outputs := make(map[wire.OutPoint]*wire.TxOut)

	for idx, in := range txToSign.TxIn {
		outputs[in.PreviousOutPoint] = fundingOutputs[idx]
	}

	outputRetriever := txscript.NewMultiPrevOutFetcher(outputs)

	for idx := range txToSign.TxIn {
		signature, err := signTxWithOneScriptSpendInputFromTapLeafInternalWithIdx(
			txToSign,
			fundingOutputs[idx],
			idx,
			outputRetriever,
			privKeys[idx],
			tapLeafs[idx],
		)
		if err != nil {
			return nil, err
		}
		signatures = append(signatures, signature)
	}

	return signatures, nil
}

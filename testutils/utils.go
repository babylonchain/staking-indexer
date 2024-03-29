package testutils

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
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

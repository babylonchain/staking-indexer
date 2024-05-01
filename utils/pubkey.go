package utils

import (
	"encoding/hex"

	"github.com/btcsuite/btcd/btcec/v2"
)

// ParseCovenantPubKeyFromHex parses public key string to btc public key
// the input should be 33 bytes
func ParseCovenantPubKeyFromHex(pkStr string) (*btcec.PublicKey, error) {
	pkBytes, err := hex.DecodeString(pkStr)
	if err != nil {
		return nil, err
	}

	pk, err := btcec.ParsePubKey(pkBytes)
	if err != nil {
		return nil, err
	}

	return pk, nil
}

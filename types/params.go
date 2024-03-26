package types

import "github.com/btcsuite/btcd/btcec/v2"

type Params struct {
	MagicBytes     []byte
	CovenantPks    []*btcec.PublicKey
	CovenantQuorum uint32
	UnbondingTime  uint16
}

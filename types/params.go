package types

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
)

type Params struct {
	Tag              []byte
	CovenantPks      []*btcec.PublicKey
	CovenantQuorum   uint32
	UnbondingTime    uint16
	MaxStakingAmount btcutil.Amount
	MinStakingAmount btcutil.Amount
	MaxStakingTime   uint16
	MinStakingTime   uint16
}

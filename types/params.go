package types

import (
	"fmt"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
)

type ParamsVersions struct {
	ParamsVersions []*Params
}

type Params struct {
	Version          uint16
	ActivationHeight int32
	StakingCap       btcutil.Amount
	Tag              []byte
	CovenantPks      []*btcec.PublicKey
	CovenantQuorum   uint32
	UnbondingTime    uint16
	UnbondingFee     btcutil.Amount
	MaxStakingAmount btcutil.Amount
	MinStakingAmount btcutil.Amount
	MaxStakingTime   uint16
	MinStakingTime   uint16
}

// GetParamsForBTCHeight Retrieve the parameters that should take into effect
// for a particular BTC height.
// Makes the assumption that the parameters are ordered by monotonically increasing `ActivationHeight`
// and that there are no overlaps between parameter activations.
func (pv *ParamsVersions) GetParamsForBTCHeight(btcHeight int32) (*Params, error) {
	// Iterate the list in reverse (i.e. decreasing ActivationHeight)
	// and identify the first element that has an activation height below
	// the specified BTC height.
	for i := len(pv.ParamsVersions) - 1; i >= 0; i-- {
		paramsVersion := pv.ParamsVersions[i]
		if paramsVersion.ActivationHeight <= btcHeight {
			return paramsVersion, nil
		}
	}
	return nil, fmt.Errorf("no version corresponds to the provided BTC height")
}

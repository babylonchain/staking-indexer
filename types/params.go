package types

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
)

type ParamsVersions struct {
	ParamsVersions []*GlobalParams
}

type GlobalParams struct {
	Version           uint16
	ActivationHeight  uint64
	StakingCap        btcutil.Amount
	CapHeight         uint64
	Tag               []byte
	CovenantPks       []*btcec.PublicKey
	CovenantQuorum    uint32
	UnbondingTime     uint16
	UnbondingFee      btcutil.Amount
	MaxStakingAmount  btcutil.Amount
	MinStakingAmount  btcutil.Amount
	MaxStakingTime    uint16
	MinStakingTime    uint16
	ConfirmationDepth uint16
}

// GetParamsForBTCHeight Retrieve the parameters that should take into effect
// for a particular BTC height.
// Makes the assumption that the parameters are ordered by monotonically increasing `ActivationHeight`
// and that there are no overlaps between parameter activations.
func (pv *ParamsVersions) GetParamsForBTCHeight(btcHeight int32) (*GlobalParams, error) {
	// Iterate the list in reverse (i.e. decreasing ActivationHeight)
	// and identify the first element that has an activation height below
	// the specified BTC height.
	for i := len(pv.ParamsVersions) - 1; i >= 0; i-- {
		paramsVersion := pv.ParamsVersions[i]
		if paramsVersion.ActivationHeight <= uint64(btcHeight) {
			return paramsVersion, nil
		}
	}
	return nil, fmt.Errorf("no version corresponds to the provided BTC height")
}

func (gp *GlobalParams) IsTimeBasedCap() bool {
	if gp.StakingCap != 0 && gp.CapHeight != 0 {
		panic(fmt.Errorf("only either of staking cap and cap height can be set"))
	}

	if gp.StakingCap == 0 && gp.CapHeight == 0 {
		panic(fmt.Errorf("either of staking cap and cap height must be set"))
	}

	return gp.CapHeight != 0
}

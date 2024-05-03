package params

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/babylonchain/babylon/btcstaking"
	bbntypes "github.com/babylonchain/babylon/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"

	"github.com/babylonchain/staking-indexer/types"
)

type ParamsRetriever interface {
	GetParamsVersions() *types.ParamsVersions
}

type LocalParamsRetriever struct {
	paramsVersions *types.ParamsVersions
}

type internalParamsVersions struct {
	ParamsVersions []*internalParams `json:"versions"`
}

type internalParams struct {
	Version          uint16         `json:"version"`
	ActivationHeight int32          `json:"activation_height"`
	StakingCap       btcutil.Amount `json:"staking_cap"`
	Tag              string         `json:"tag"`
	CovenantPks      []string       `json:"covenant_pks"`
	CovenantQuorum   uint32         `json:"covenant_quorum"`
	UnbondingTime    uint16         `json:"unbonding_time"`
	UnbondingFee     btcutil.Amount `json:"unbonding_fee"`
	MaxStakingAmount btcutil.Amount `json:"max_staking_amount"`
	MinStakingAmount btcutil.Amount `json:"min_staking_amount"`
	MaxStakingTime   uint16         `json:"max_staking_time"`
	MinStakingTime   uint16         `json:"min_staking_time"`
}

func NewLocalParamsRetriever(filePath string) (*LocalParamsRetriever, error) {
	contents, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read params file %s: %w", filePath, err)
	}

	var pv internalParamsVersions
	err = json.Unmarshal(contents, &pv)
	if err != nil {
		return nil, fmt.Errorf("invalid params content: %w", err)
	}

	paramsVersions := &types.ParamsVersions{
		ParamsVersions: make([]*types.Params, 0),
	}
	// Define prior params to compare against
	var previousParams *internalParams = nil
	for _, p := range pv.ParamsVersions {
		if len(p.Tag) != btcstaking.MagicBytesLen {
			return nil, fmt.Errorf("invalid tag length, expected %d, got %d", btcstaking.MagicBytesLen, len(p.Tag))
		}

		if len(p.CovenantPks) == 0 {
			return nil, fmt.Errorf("empty covenant public keys")
		}
		if p.CovenantQuorum > uint32(len(p.CovenantPks)) {
			return nil, fmt.Errorf("covenant quorum cannot be more than the amount of covenants")
		}

		covPks := make([]*btcec.PublicKey, len(p.CovenantPks))
		for i, covPk := range p.CovenantPks {
			pk, err := bbntypes.NewBIP340PubKeyFromHex(covPk)
			if err != nil {
				return nil, fmt.Errorf("invalid covenant public key %s: %w", covPk, err)
			}
			covPks[i] = pk.MustToBTCPK()
		}

		if p.MaxStakingAmount <= p.MinStakingAmount {
			return nil, fmt.Errorf("max-staking-amount must be larger than min-staking-amount")
		}

		if p.MaxStakingTime <= p.MinStakingTime {
			return nil, fmt.Errorf("max-staking-time must be larger than min-staking-time")
		}

		if p.ActivationHeight <= 0 {
			return nil, fmt.Errorf("activation height should be positive")
		}
		if p.StakingCap <= 0 {
			return nil, fmt.Errorf("staking cap should be positive")
		}

		// Check previous parameters conditions
		if previousParams != nil {
			if p.Version != previousParams.Version+1 {
				return nil, fmt.Errorf("versions should be monotonically increasing by 1")
			}
			if p.StakingCap < previousParams.StakingCap {
				return nil, fmt.Errorf("staking cap cannot be decreased in later versions")
			}
			if p.ActivationHeight < previousParams.ActivationHeight {
				return nil, fmt.Errorf("activation height cannot be overlapping between earlier and later versions")
			}
			previousParams = p
		}

		paramsVersions.ParamsVersions = append(paramsVersions.ParamsVersions, &types.Params{
			Version:          p.Version,
			StakingCap:       p.StakingCap,
			ActivationHeight: p.ActivationHeight,
			Tag:              []byte(p.Tag),
			CovenantPks:      covPks,
			CovenantQuorum:   p.CovenantQuorum,
			UnbondingTime:    p.UnbondingTime,
			UnbondingFee:     p.UnbondingFee,
			MaxStakingAmount: p.MaxStakingAmount,
			MinStakingAmount: p.MinStakingAmount,
			MaxStakingTime:   p.MaxStakingTime,
			MinStakingTime:   p.MinStakingTime,
		})
	}

	return &LocalParamsRetriever{paramsVersions: paramsVersions}, nil
}

func (lp *LocalParamsRetriever) GetParamsVersions() *types.ParamsVersions {
	return lp.paramsVersions
}

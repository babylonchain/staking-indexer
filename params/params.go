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
	GetParams() *types.Params
}

type LocalParamsRetriever struct {
	params *types.Params
}

func NewLocalParamsRetriever(filePath string) (*LocalParamsRetriever, error) {
	type internalParams struct {
		Tag              string         `json:"tag"`
		CovenantPks      []string       `json:"covenant_pks"`
		CovenantQuorum   uint32         `json:"covenant_quorum"`
		UnbondingTime    uint16         `json:"unbonding_time"`
		MaxStakingAmount btcutil.Amount `json:"max_staking_amount"`
		MinStakingAmount btcutil.Amount `json:"min_staking_amount"`
		MaxStakingTime   uint16         `json:"max_staking_time"`
		MinStakingTime   uint16         `json:"min_staking_time"`
	}

	contents, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read params file %s: %w", filePath, err)
	}

	var p internalParams
	err = json.Unmarshal(contents, &p)
	if err != nil {
		return nil, fmt.Errorf("invalid params content: %w", err)
	}

	if len(p.Tag) != btcstaking.MagicBytesLen {
		return nil, fmt.Errorf("invalid tag length, expected %d, got %d", btcstaking.MagicBytesLen, len(p.Tag))
	}

	if len(p.CovenantPks) == 0 {
		return nil, fmt.Errorf("empty covenant public keys")
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

	params := &types.Params{
		Tag:              []byte(p.Tag),
		CovenantPks:      covPks,
		CovenantQuorum:   p.CovenantQuorum,
		UnbondingTime:    p.UnbondingTime,
		MaxStakingAmount: p.MaxStakingAmount,
		MinStakingAmount: p.MinStakingAmount,
		MaxStakingTime:   p.MaxStakingTime,
		MinStakingTime:   p.MinStakingTime,
	}

	return &LocalParamsRetriever{params: params}, nil
}

func (lp *LocalParamsRetriever) GetParams() *types.Params {
	return lp.params
}

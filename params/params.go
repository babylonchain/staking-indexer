package params

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/babylonchain/babylon/btcstaking"
	bbntypes "github.com/babylonchain/babylon/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"

	"github.com/babylonchain/staking-indexer/proto"
	"github.com/babylonchain/staking-indexer/types"
)

type ParamsRetriever interface {
	GetParams() *types.Params
}

type LocalParamsRetriever struct {
	params *types.Params
}

func NewLocalParamsRetriever(filePath string) (*LocalParamsRetriever, error) {
	contents, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read params file %s: %w", filePath, err)
	}

	var p proto.GlobalParams
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

	if len(p.FinalityProviderPks) == 0 {
		return nil, fmt.Errorf("empty finality provider public keys")
	}

	fpPks := make([]*btcec.PublicKey, len(p.FinalityProviderPks))
	for i, fpPk := range p.FinalityProviderPks {
		pk, err := bbntypes.NewBIP340PubKeyFromHex(fpPk)
		if err != nil {
			return nil, fmt.Errorf("invalid finality provider public key %s: %w", fpPk, err)
		}
		fpPks[i] = pk.MustToBTCPK()
	}

	if p.MaxStakingAmount <= p.MinStakingAmount {
		return nil, fmt.Errorf("max-staking-amount must be larger than min-staking-amount")
	}

	if p.MaxStakingTime <= p.MinStakingTime {
		return nil, fmt.Errorf("max-staking-time must be larger than min-staking-time")
	}

	params := &types.Params{
		Tag:                 []byte(p.Tag),
		CovenantPks:         covPks,
		FinalityProviderPks: fpPks,
		CovenantQuorum:      p.CovenantQuorum,
		UnbondingTime:       uint16(p.UnbondingTime),
		MaxStakingAmount:    btcutil.Amount(p.MaxStakingAmount),
		MinStakingAmount:    btcutil.Amount(p.MinStakingAmount),
		MaxStakingTime:      uint16(p.MaxStakingTime),
		MinStakingTime:      uint16(p.MinStakingTime),
	}

	return &LocalParamsRetriever{params: params}, nil
}

func (lp *LocalParamsRetriever) GetParams() *types.Params {
	return lp.params
}

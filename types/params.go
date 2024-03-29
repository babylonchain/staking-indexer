package types

import (
	bbntypes "github.com/babylonchain/babylon/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"

	"github.com/babylonchain/staking-indexer/proto"
)

type Params struct {
	Tag                 []byte
	CovenantPks         []*btcec.PublicKey
	FinalityProviderPks []*btcec.PublicKey
	CovenantQuorum      uint32
	UnbondingTime       uint16
	MaxStakingAmount    btcutil.Amount
	MinStakingAmount    btcutil.Amount
	MaxStakingTime      uint16
	MinStakingTime      uint16
}

func (p *Params) ToProto() *proto.GlobalParams {
	covPksStr := make([]string, len(p.CovenantPks))
	for i, pk := range p.CovenantPks {
		covPksStr[i] = bbntypes.NewBIP340PubKeyFromBTCPK(pk).MarshalHex()
	}

	fpPksStr := make([]string, len(p.FinalityProviderPks))
	for i, pk := range p.FinalityProviderPks {
		fpPksStr[i] = bbntypes.NewBIP340PubKeyFromBTCPK(pk).MarshalHex()
	}

	return &proto.GlobalParams{
		Tag:                 string(p.Tag),
		CovenantPks:         covPksStr,
		FinalityProviderPks: fpPksStr,
		CovenantQuorum:      p.CovenantQuorum,
		UnbondingTime:       uint32(p.UnbondingTime),
		MaxStakingAmount:    uint64(p.MaxStakingAmount),
		MinStakingAmount:    uint64(p.MinStakingAmount),
		MaxStakingTime:      uint32(p.MaxStakingTime),
		MinStakingTime:      uint32(p.MinStakingTime),
	}
}

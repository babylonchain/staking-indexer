package params

import (
	"encoding/json"
	"os"

	"github.com/babylonchain/networks/parameters/parser"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
)

type ParamsRetriever interface {
	VersionedParams() *parser.ParsedGlobalParams
}

type GlobalParamsRetriever struct {
	paramsVersions *parser.ParsedGlobalParams
}

type GlobalParams struct {
	Versions []*VersionedGlobalParams `json:"versions"`
}

type VersionedGlobalParams struct {
	Version           uint64   `json:"version"`
	ActivationHeight  uint64   `json:"activation_height"`
	StakingCap        uint64   `json:"staking_cap"`
	CapHeight         uint64   `json:"cap_height"`
	Tag               string   `json:"tag"`
	CovenantPks       []string `json:"covenant_pks"`
	CovenantQuorum    uint64   `json:"covenant_quorum"`
	UnbondingTime     uint64   `json:"unbonding_time"`
	UnbondingFee      uint64   `json:"unbonding_fee"`
	MaxStakingAmount  uint64   `json:"max_staking_amount"`
	MinStakingAmount  uint64   `json:"min_staking_amount"`
	MaxStakingTime    uint64   `json:"max_staking_time"`
	MinStakingTime    uint64   `json:"min_staking_time"`
	ConfirmationDepth uint64   `json:"confirmation_depth"`
}

type ParsedGlobalParams struct {
	Versions []*ParsedVersionedGlobalParams
}

type ParsedVersionedGlobalParams struct {
	Version           uint64
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

func NewGlobalParamsRetriever(filePath string) (*GlobalParamsRetriever, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var globalParams parser.GlobalParams
	err = json.Unmarshal(data, &globalParams)
	if err != nil {
		return nil, err
	}

	parsedGlobalParams, err := parser.ParseGlobalParams(&globalParams)
	if err != nil {
		return nil, err
	}

	return &GlobalParamsRetriever{paramsVersions: parsedGlobalParams}, nil
}

func (lp *GlobalParamsRetriever) VersionedParams() *parser.ParsedGlobalParams {
	return lp.paramsVersions
}

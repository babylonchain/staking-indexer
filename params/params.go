package params

import (
	"github.com/btcsuite/btcd/btcec/v2"

	"github.com/babylonchain/staking-indexer/types"
)

var covenantPrivKey, _ = btcec.NewPrivateKey()

type ParamsRetriever interface {
	GetParams() (*types.Params, error)
}

type LocalParamsRetriever struct {
	params *types.Params
}

func NewLocalParamsRetriever() *LocalParamsRetriever {
	magicBytes := []byte("1234")
	covenantPks := []*btcec.PublicKey{covenantPrivKey.PubKey()}
	covenantQuorum := uint32(1)
	unbondingTime := uint16(1000)

	return &LocalParamsRetriever{params: &types.Params{
		MagicBytes:     magicBytes,
		CovenantPks:    covenantPks,
		CovenantQuorum: covenantQuorum,
		UnbondingTime:  unbondingTime,
	}}
}

func (lp *LocalParamsRetriever) GetParams() (*types.Params, error) {
	return lp.params, nil
}

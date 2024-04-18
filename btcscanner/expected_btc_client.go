package btcscanner

import (
	"github.com/babylonchain/staking-indexer/types"
)

type Client interface {
	GetTipHeight() (uint64, error)
	GetBlockByHeight(height uint64) (*types.IndexedBlock, error)
}

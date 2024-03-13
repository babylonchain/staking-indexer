package btcscanner

import vtypes "github.com/babylonchain/vigilante/types"

type Scanner interface {
	Start() error
	Stop() error

	ConfirmedBlocksChan() chan *vtypes.IndexedBlock
}

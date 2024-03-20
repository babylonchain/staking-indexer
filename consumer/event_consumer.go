package consumer

import "github.com/babylonchain/staking-indexer/types"

type EventConsumer interface {
	Start() error
	PushStakingEvent(ev *types.ActiveStakingEvent) error
	Stop() error
}

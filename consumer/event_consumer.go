package consumer

import "github.com/babylonchain/staking-indexer/types"

type EventConsumer interface {
	Start() error
	PushStakingEvent(ev *types.ActiveStakingEvent) error
	PushUnbondingEvent(ev *types.UnbondingStakingEvent) error
	PushWithdrawEvent(ev *types.WithdrawStakingEvent) error
	Stop() error
}

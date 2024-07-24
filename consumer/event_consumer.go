package consumer

import (
	// "github.com/babylonchain/staking-queue-client/client"
	"github.com/scalarorg/staking-queue-client/client"
)

type EventConsumer interface {
	Start() error
	PushStakingEvent(ev *client.ActiveStakingEvent) error
	PushUnbondingEvent(ev *client.UnbondingStakingEvent) error
	PushWithdrawEvent(ev *client.WithdrawStakingEvent) error
	PushBtcInfoEvent(ev *client.BtcInfoEvent) error
	Stop() error

	// SCALAR
	PushVaultEvent(ev *client.ActiveVaultEvent) error
	PushBurningEvent(ev *client.BurningVaultEvent) error
	PushWithdrawVaultEvent(ev *client.WithdrawVaultEvent) error
}

package consumer

import (
	"fmt"

	"github.com/babylonchain/staking-indexer/types"
)

type DumbConsumer struct {
}

func (dc *DumbConsumer) PushStakingEvent(ev *types.ActiveStakingEvent) error {
	fmt.Printf("tx hash of the staking event is %s", ev.StakingTxHex)

	return nil
}

func (dc *DumbConsumer) PushUnbondingEvent(ev *types.UnbondingStakingEvent) error {
	fmt.Printf("staking tx hash of the unbonding event is %s", ev.StakingTxHashHex)

	return nil
}

func (dc *DumbConsumer) PushWithdrawEvent(ev *types.WithdrawStakingEvent) error {
	fmt.Printf("staking tx hash of the withdraw event is %s", ev.StakingTxHashHex)

	return nil
}

func (dc *DumbConsumer) Start() error {
	return nil
}

func (dc *DumbConsumer) Stop() error {
	return nil
}

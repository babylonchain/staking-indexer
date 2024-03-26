package types

import (
	"encoding/hex"

	"github.com/babylonchain/babylon/btcstaking"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

type EventType int

const (
	ActiveStakingEventType EventType = iota
	UnbondingStakingEventType
	WithdrawStakingEventType
	ExpiredStakingEventType
)

type ActiveStakingEvent struct {
	EventType             EventType `json:"event_type"` // always 1. ActiveStakingEventType
	StakingTxHex          string    `json:"staking_tx_hex"`
	StakerPkHex           string    `json:"staker_pk_hex"`
	FinalityProviderPkHex string    `json:"finality_provider_pk_hex"`
	StakingValue          uint64    `json:"staking_value"`
	StakingStartHeight    uint64    `json:"staking_start_height"`
	StakingTimeLock       uint16    `json:"staking_timelock"`
}

type UnbondingStakingEvent struct {
	EventType            EventType `json:"event_type"` // always 2. UnbondingStakingEventType
	StakingTxHash        string    `json:"staking_tx_hash"`
	UnbondingStartHeight uint64    `json:"unbonding_start_height"`
	UnbondingTimeLock    uint16    `json:"unbonding_timelock"`
}

type WithdrawStakingEvent struct {
	EventType     EventType `json:"event_type"` // always 3. WithdrawStakingEventType
	StakingTxHash string    `json:"staking_tx_hash"`
}

func StakingDataToEvent(stakingData *btcstaking.ParsedV0StakingTx, txHash chainhash.Hash, blockHeight uint64) *ActiveStakingEvent {
	return &ActiveStakingEvent{
		EventType:             ActiveStakingEventType,
		StakingTxHex:          txHash.String(),
		StakerPkHex:           hex.EncodeToString(stakingData.OpReturnData.StakerPublicKey.Marshall()),
		FinalityProviderPkHex: hex.EncodeToString(stakingData.OpReturnData.FinalityProviderPublicKey.Marshall()),
		StakingValue:          uint64(stakingData.StakingOutput.Value),
		StakingStartHeight:    blockHeight,
		StakingTimeLock:       stakingData.OpReturnData.StakingTime,
	}
}

func NewUnbondingStakingEvent(stakingTxHash *chainhash.Hash, unbondingStartHeight uint64, unbondingTimeLock uint16) *UnbondingStakingEvent {
	return &UnbondingStakingEvent{
		EventType:            UnbondingStakingEventType,
		StakingTxHash:        stakingTxHash.String(),
		UnbondingStartHeight: unbondingStartHeight,
		UnbondingTimeLock:    unbondingTimeLock,
	}
}

func NewWithdrawEvent(stakingTxHash *chainhash.Hash) *WithdrawStakingEvent {
	return &WithdrawStakingEvent{
		EventType:     UnbondingStakingEventType,
		StakingTxHash: stakingTxHash.String(),
	}
}

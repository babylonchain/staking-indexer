package types

import (
	"encoding/hex"

	"github.com/babylonchain/babylon/btcstaking"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

type EventType int32

const (
	EventTypeStaking EventType = 0
)

type ActiveStakingEvent struct {
	EventType               EventType `json:"event_type"`
	StakingTxHex            string    `json:"staking_tx_hex"`
	StakerPkHex             string    `json:"staker_pk_hex"`
	FinalityProviderPkHex   string    `json:"finality_provider_pk_hex"`
	StakingValue            uint64    `json:"staking_value"`
	StakingStartBlockHeight uint64    `json:"staking_start_block_height"`
	StakingLength           uint16    `json:"staking_length"`
}

func StakingDataToEvent(stakingData *btcstaking.ParsedV0StakingTx, txHash chainhash.Hash, blockHeight uint64) *ActiveStakingEvent {
	return &ActiveStakingEvent{
		EventType:               EventTypeStaking,
		StakingTxHex:            txHash.String(),
		StakerPkHex:             hex.EncodeToString(stakingData.OpReturnData.StakerPublicKey.Marshall()),
		FinalityProviderPkHex:   hex.EncodeToString(stakingData.OpReturnData.FinalityProviderPublicKey.Marshall()),
		StakingValue:            uint64(stakingData.StakingOutput.Value),
		StakingStartBlockHeight: blockHeight,
		StakingLength:           stakingData.OpReturnData.StakingTime,
	}
}

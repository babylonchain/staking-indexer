package types

type ActiveStakingEvent struct {
	EventType               uint8  `json:"event_type"`
	StakingTxHex            string `json:"staking_tx_hex"`
	StakerPkHex             string `json:"staker_pk_hex"`
	FinalityProviderPkHex   string `json:"finality_provider_pk_hex"`
	StakingValue            uint64 `json:"staking_value"`
	StakingStartBlockHeight uint64 `json:"staking_start_block_height"`
	StakingLength           uint16 `json:"staking_length"`
}

type UnbondStakingEvent struct {
	EventType     uint8  `json:"event_type"` // This is hardcoded value
	StakingTxHash string `json:"staking_tx_hash"`
	UnboundAmount uint64 `json:"unbonded_amount"`
	// type of unbonded tx, {convenient-unbond, withdrawal, slash}
	UnbondType            string `json:"unbond_type"`
	StakerPkHex           string `json:"staker_pk_hex"`
	FinalityProviderPkHex string `json:"finality_provider_pk_hex"`

	UnbondBtcBlockHeight uint64 `json:"unbond_btc_block_height"`
	UnbondBtcTxHex       string `json:"unbond_btc_tx_hex"`
}

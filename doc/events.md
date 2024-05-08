# Events

The Staking Indexer observes and emits three types of events, `StakingEvent`,
`UnbondingEvent`, `WithdrawalEvent`, each defined as follows.

### Staking Event

```go
type ActiveStakingEvent struct {
	EventType             EventType `json:"event_type"` // always 1. ActiveStakingEventType
	StakingTxHashHex      string    `json:"staking_tx_hash_hex"`
	StakerPkHex           string    `json:"staker_pk_hex"`
	FinalityProviderPkHex string    `json:"finality_provider_pk_hex"`
	StakingValue          uint64    `json:"staking_value"`
	StakingStartHeight    uint64    `json:"staking_start_height"`
	StakingStartTimestamp int64     `json:"staking_start_timestamp"`
	StakingTimeLock       uint64    `json:"staking_timelock"`
	StakingOutputIndex    uint64    `json:"staking_output_index"`
	StakingTxHex          string    `json:"staking_tx_hex"`
	IsOverflow            bool      `json:"is_overflow"`
}
```

### Unbonding Event

```go
type UnbondingStakingEvent struct {
	EventType               EventType `json:"event_type"` // always 2. UnbondingStakingEventType
	StakingTxHashHex        string    `json:"staking_tx_hash_hex"`
	UnbondingStartHeight    uint64    `json:"unbonding_start_height"`
	UnbondingStartTimestamp int64     `json:"unbonding_start_timestamp"`
	UnbondingTimeLock       uint64    `json:"unbonding_timelock"`
	UnbondingOutputIndex    uint64    `json:"unbonding_output_index"`
	UnbondingTxHex          string    `json:"unbonding_tx_hex"`
	UnbondingTxHashHex      string    `json:"unbonding_tx_hash_hex"`
}
```

### Withdrawal Event

```go
type WithdrawStakingEvent struct {
    EventType        EventType `json:"event_type"` // always 3. WithdrawStakingEventType
    StakingTxHashHex string    `json:"staking_tx_hash_hex"`
}
```

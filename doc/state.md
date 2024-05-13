# System State

The staking indexer relies on a set of stores to persist the system state.

### Staking Transaction Store

The staking transaction store is to store the parsed staking transaction data.
This is used to identify future unbonding/withdrawal transactions.
The key is the transaction hash and the value is defined as the follows.

```protobuf
message StakingTransaction {
  // transaction_bytes is the full tx data
  bytes transaction_bytes = 1;

  uint32 staking_output_idx = 2;

  // inclusion_height is the height the tx included
  // on BTC
  uint64 inclusion_height = 3;

  // staking info
  bytes staker_pk = 4;
  bytes finality_provider_pk = 5;
  uint32 staking_time = 6;

  // Indicate if the staking tx would exceed the staking cap.
  bool is_overflow = 7;
  // The staking amount
  uint64 staking_value = 8;
}
```

### Unbonding Transaction Store

The unbonding transaction store is to store the unbonding transaction record.
This is used to identify future withdrawal transactions that spend the 
unbonding transaction output.
The key is the transaction hash and the value is defined as the follows.

```protobuf
message UnbondingTransaction {
    // transaction_bytes is the full tx data
    bytes transaction_bytes = 1;
    // staking_tx_hash is the hash of the staking tx
    // that the unbonding tx spend
    bytes staking_tx_hash = 2;
}
```

### Indexer State Store

The indexer state store is to record the last processed BTC height.
This helps the indexer bootstrap.

### Confirmed TVL Store

The confirmed TVL store is to store the TVL calculated based on the existing 
transactions (both staking and unbonding transactions).
This is used to identify whether a staking transaction is active or overflow.

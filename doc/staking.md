# Staking

In this document, we explore how the staking indexer transactions of the
Bitcoin Staking protocol.

## Preliminaries

### Staking Transactions

The Bitcoin Staking protocol classifies a transaction into three types:
- *Staking Transaction*: A transaction that contains an output that commits to
  the self-custodial Bitcoin Staking script. The value specified in this output
  corresponds to the Bitcoin staked. The Bitcoin Staking script defines
  conditions for timelock based withdrawal, on-demand unbonding, and slashing.
- *Unbonding Transaction*: A transaction that consumes the Bitcoin Staking
  output and commits to the self-custodial Bitcoin Unbonding script. This
  script contains conditions for timelock based withdrawal with the timelock
  being lower than the one in the staking script and slashing.

More details on the Bitcoin Staking transactions and scripts can be found in
this [reference](https://github.com/babylonchain/babylon/blob/v0.8.5/docs/staking-script.md).

### Staking Parameters

The phase-1 parameters are governance parameters that specify what constitutes
a valid staking transaction that should be considered as an active one. They
are maintained by Babylon and are timestamped on Bitcoin by a Bitcoin
governance wallet owned by it. They are additionally included in a GitHub
registry for easy retrieval and timestamp verification.

There are different versions of parameters with each one having a particular
Bitcoin activation height. Each version contains the following:
```json
{
  "version": <params_version>,
  "activation_height": <bitcoin_activation_height>,
  "staking_cap": <satoshis_staking_cap_of_version>,
  "tag": "<magic_bytes_to_identify_staking_txs>",
  "covenant_pks": [
    "<covenant_btc_pk1>",
    "<covenant_btc_pk2>",
    ...
  ],
  "covenant_quorum": <covenant_quorum>,
  "unbonding_time": <unbonding_time_btc_blocks>,
  "unbonding_fee": <unbonding_fee_satoshis>,
  "max_staking_amount": <max_staking_amount_satoshis>,
  "min_staking_amount": <min_staking_amount_satoshis>,
  "max_staking_time": <max_staking_time_btc_blocks>,
  "min_staking_time": <min_staking_time_btc_blocks>
}
```

The staking indexer evaluates staking transactions based on the above
parameters (e.g. the amount should be on the defined range). A parameters
version is chosen based on the height the *staking transaction* has been
included in (i.e. unbonding transactions are evaluated based on the parameters
of the staking transaction which they unbond). Notably, the parameters specify
a staking cap value, which defines the maximum amount of Bitcoin stake the
system considers active at any moment.


## Staking Transactions Processing

In the phase-1 system, Bitcoin serves as the only source of truth about the
events of the system. Events correspond to Bitcoin Staking and Unbonding
transactions and based on their ordering in the Bitcoin ledger we
can identify the parameters they should be validated against to be accepted.

The system described below, assumes that it is dealing with irreversible
transactions, i.e. transactions that are deep enough on the Bitcoin ledger that
their reversal is highly improbable. Therefore, no forking considerations are
taken into account. Further, the system is replayable, meaning that the Bitcoin
history can be replayed in order to reach the same state conclusions,
as long as a fork larger than the confirmation parameter has happened.

The system processes Bitcoin blocks and transactions in the order they appear
on Bitcoin and are decoded based on the spec [here](/doc/extract_tx_data.md).
In the below sections,
we define the state of the system and its transitions,
as well as the validation conditions for the different transactions.

### State

The system maintains two pieces of state:
- *Active Staking Transactions*: A list of active staking transactions. It used
  to identify the current amount of Bitcoin that is locked in the system.
- *Overflow Staking Transactions*: A list of valid staking transactions that
  are over the staking cap. This list is only maintained to keep track of the
  entire system state as well as provide more novice users with enough
  information to on-demand unbond their stake.

### New Staking Transactions

We implement a first-come-first-serve process for activating
valid staking transactions based on the system parameters. If a transaction
contains a valid set of parameters, it is classified into `Active` and
`Overflow` depending on whether it can fit to the current parameter's staking
cap or not.

For Bitcoin transactions that adhere to the Staking transaction format,
we retrieve the Staking Parameters that correspond to the Bitcoin block the
transaction has been included in, by finding the first set of parameters `v_n`
for which `v_n.ActivationHeight <= blockHeight and
(v_(n+1) == nil or v_(n+1).ActivationHeight > blockHeight)`.

Based on these parameters, we perform the following checks:
- `v_n.MinStakingAmount <= StakingTransaction.StakeAmount <=
  v_n.MaxStakingAmount`
- `v_n.MinStakingTime <= StakingTransaction.StakingTime <=
  v_n.MaxStakingTime`
- `v_n.Tag == StakingTransaction.Tag`
- `v_n.CovenantPks == StakingTransaction.CovenantPks`
- `v_n.CovenantQuorum == StakingTransaction.CovenantQuorum`

The above checks verify that the staking transaction is a valid formatted
staking transaction based on the parameters. To further identify whether the
transaction should be an active one or it goes over the staking cap, we perform
the following check:
- `sum(State.ActiveStakingTransactions[].StakeAmount) +
  StakingTransaction.StakingAmount <= v_n.StakingCap`

If the transaction satisfied the above check, it is added to the active staking
transactions list. Otherwise, it is classified as overflow.

#### Timelock Expiration

Staking transactions contain a timelock that can expire. The indexer monitors
for staking transactions that expire upon each block before moving forward to
its processing. In case a transaction in the active staking transactions list
expires, then it is removed from the active staking transactions list, making
space for new staking transactions.

> Note: This has not been implemented yet.

### Unbonding Transactions

For Bitcoin transactions that adhere to the Unbonding transaction format,
we check whether the staking transaction which they unbond is included in the
set of active/overflow staking transactions, and if so,
we retrieve the corresponding transaction. 

We verify the unbonding transaction based on the staking parameters defined for
the *staking transaction*, as the staking transaction has already specified the
covenant public keys that should be used. The parameters `v_n`, are retrieved the same
way as with the Staking Transaction, and we perform the following
verifications:
- `v_n.CovenantPks == UnbondingTransaction.CovenantPks`
- `v_n.CovenantQuorum == UnbondingTransaction.CovenantQuorum`
- `UnbondingTransaction.Signatures.ContainQuorum(v_n.CovenantPks,
  v_n.CovenantQuorum`).
- `v_n.UnbondingFee == UnbondingTransaction.UnbondingFee`
- `v_n.UnbondingTime == UnbondingTransaction.UnbondingTime`

If all the conditions are valid, the staking transaction is removed from the
active/overflow staking transactions set. This effectivelly means,
that if the staking transaction was active and the staking cap
had previously been filled, now there is space for new staking transactions.
These staking transactions need to come later than the unbonding transaction.

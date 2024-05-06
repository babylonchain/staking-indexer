# Staking Activation

In this document, we explore how to identify the activation status of a staking
transaction.

In the phase-1 system, Bitcoin serves as the only source of truth about the
events of the system. Events correspond to Bitcoin Staking and Unbonding
transactions and based on their ordering in the Bitcoin ledger we can identify
the parameters they should be validated against to be accepted.

The below system description assumes that it is dealing with irreversible
transactions, i.e. transactions that are deep enough on the Bitcoin ledger that
their reversal is highly improbable. Therefore, no forking considerations are
taken into account. Further, the system is replayable, meaning that the Bitcoin
history can be replayed in order to reach the same state conclusions,
as long as a fork larger than the confirmation parameter has happened.

The system processes Bitcoin blocks and transactions in the order they appear
on Bitcoin.

## State

The system maintains a state corresponding to the active staking transactions.
Based on those, the system can identify which new staking transactions should
be accepted or not.

## New Staking Transactions

We implement a first-come first-serve process for activating
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

## Unbonding Transactions

For Bitcoin transactions that adhere to the Unbonding transaction format,
we check whether the staking transaction which they unbond is included in the
set of active staking transactions, and if so, we retrieve the corresponding
transaction. 

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
active staking transactions set. This effectivelly means, that if the staking cap
had previously been filled, now there is space for new staking transactions.
These staking transactions, need to come later than the unbonding transaction.

### Naturally Expiring Transactions

Staking transactions contain a timelock that can expire. The indexer monitors
for staking transactions that expire upon each block before moving forward to
its processing. In case a transaction in the active staking transactions list
expires, then it is removed from the active staking transactions list, making
space for new staking transactions.

> Note: This has not been implemented yet.

### Dealing With Invalid/Overflow Transactions

The above system description does not maintain Overflow or Inactive
transactions in its state, as they are not related to the stake activation
procedure. However, the staking indexer makes the design decision to maintain
Overflow transactions in its internal state to enable and monitor for their
unbonding in order to provide a better user experience for stakers that didn't
make it into the staking cap.

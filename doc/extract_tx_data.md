# Extract Transaction Data

The indexer reads confirmed blocks from the poller in a monotonically 
increasing manner. This is to ensure the causal order of the events. 

The indexer performs the following steps to process transactions:
1. Try to parse it as a staking transaction. The staking data and parsing 
   details can be found in [the staking tx spec](/doc/staking.md#new-staking-transactions). 
   1. If a staking transaction is found, emit `ActiveStakingEvent` and 
      persist the parsed staking transaction data in the database. The
      `ActiveStakingEvent` specifies whether the transaction is `Active` or
      `Overflow`.
2. Check whether the transaction spends any previous staking transactions 
   stored in the database. 
   1. If a spending transaction is found, check whether it is a valid unbonding 
      transaction. The definition of an unbonding transaction and validation 
      details can be found in [the unbonding tx
      spec](/doc/staking.md#unbonding-transactions).
      1. If a valid unbonding transaction is found, emit `UnbondingEvent` and 
         persist the unbonding transaction data in the database. This happens
         for both active and overflow staking transactions.
      2. If the transaction is found to unlock the unbonding path but does 
         not pass the validation, an alarm will be raised as this indicates 
         that the covenant committee has signed on an invalid unbonding 
         transaction.
   2. If the spending transaction does not unlock the unbonding path, then 
      check whether it unlocks the time-lock path. If so, emit 
      `WithdrawEvent`. Otherwise, raise an alarm as the transaction is spent 
      from an unexpected path. This happens for both active and overflow
      staking transactions.
3. If the transaction does not spend any stored staking transactions, then 
   check whether it spends any stored unbonding transactions.
   1. If a spending transaction is found, then check whether it unlocks the 
      output via the time-lock path. If so, emit `WithdrawEvent`.
   2. Otherwise, raise an alarm as the transaction is spent from an 
      unexpected path.

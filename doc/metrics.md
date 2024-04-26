# Metrics

The staking indexer contains a Prometheus server which records the following 
metrics:

## Stats

* `startBtcHeight`: The BTC height at which the indexer starts scanning

* `lastProcessedBtcHeight`: Last processed BTC height

* `lastFoundStakingTx`: The info of the last found staking transaction 
including block height, transaction id, staker public key, staking value, 
  staking time, and finality provider public key

* `lastFoundUnbondingTx`: The info of the last found unbonding transaction 
including block height, transaction id, and staking transaction id

* `lastFoundWithdrawTxFromStaking`: The info of the last found withdraw 
transaction from staking including block height, transaction id, and staking transaction id

* `lastFoundWithdrawTxFromUnbonding`: The info of the last found withdraw 
transaction from unbonding including block height, transaction id, unbonding transaction id, and staking transaction id

* `totalStakingTxs`: Total number of staking transactions

* `totalUnbondingTxs`: Total number of unbonding transactions

* `totalWithdrawTxsFromStaking`: Total number of withdraw transactions from 
staking path

* `totalWithdrawTxsFromUnbonding`: Total number of withdraw transactions from 
unbonding path

## Alerts
* `failedProcessingStakingTxsCounter`: Total number of failed staking txs 
during processing

* `failedCheckingUnbondingTxsCounter`: Total number of failed unbonding txs 
during checking

* `failedProcessingUnbondingTxsCounter`: Total number of failed unbonding txs 
during processing

* `failedProcessingWithdrawTxsFromStakingCounter`: Total number of failed 
withdraw txs from staking during processing

* `failedProcessingWithdrawTxsFromUnbondingCounter`: Total number of failed 
withdraw txs from unbonding during processing

* `irregularTxsFromStakingCounter`: Total number of irregular transactions that 
spend a staking tx

* `irregularTxsFromUnbondingCounter`: Total number of irregular transactions 
that spend a unbonding tx

* `invalidUnbondingTxsCounter`: Total number of invalid unbonding transactions

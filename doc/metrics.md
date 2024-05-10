# Metrics

The staking indexer contains a Prometheus server which records the following 
metrics:

## Stats

* `startBtcHeight`: The BTC height from which the indexer starts scanning

* `lastProcessedBtcHeight`: Last processed BTC height

* `lastFoundStakingTx`: The info of the last found staking transaction 

* `lastFoundUnbondingTx`: The info of the last found unbonding transaction 

* `lastFoundWithdrawTxFromStaking`: The info of the last found withdrawal
  transaction spending a previous staking transaction 

* `lastFoundWithdrawTxFromUnbonding`: The info of the last found withdrawal 
  transaction spending a previous unbonding transaction
 
* `lastCalculatedTvl`: The value of the last calculated TVL in satoshis

* `totalStakingTxs`: Total number of staking transactions

* `totalUnbondingTxs`: Total number of unbonding transactions

* `totalWithdrawTxsFromStaking`: Total number of withdrawal transactions from 
  the staking path

* `totalWithdrawTxsFromUnbonding`: Total number of withdrawal transactions 
  from the unbonding path

## Alerts

The following alerts indicate systematic errors are happening and the
service operator should take actions of checking other components or the 
global parameters.

* `failedProcessingStakingTxsCounter`: Total number of failures when 
  processing valid staking transactions

* `failedVerifyingUnbondingTxsCounter`: Total number of failures when 
  verifying unbonding txs 

* `failedProcessingUnbondingTxsCounter`: Total number of failures when 
  processing valid unbonding transactions

* `failedProcessingWithdrawTxsFromStakingCounter`: Total number of failures 
  when processing valid withdrawal transactions from staking 

* `failedProcessingWithdrawTxsFromUnbondingCounter`: Total number of 
  failures when processing valid withdrawal transactions from unbonding

* `irregularTxsFromStakingCounter`: Total number of irregular transactions 
  that spend a staking tx

* `irregularTxsFromUnbondingCounter`: Total number of irregular transactions 
  that spend an unbonding tx

* `invalidUnbondingTxsCounter`: Total number of invalid unbonding transactions

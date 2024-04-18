package indexer

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	/* statistics */

	startBtcHeight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "si_start_btc_height",
		Help: "The BTC height at which the indexer starts scanning",
	})

	lastProcessedBtcHeight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "si_last_processed_btc_height",
		Help: "Last processed BTC height",
	})

	lastFoundStakingTx = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "si_last_found_staking_tx",
			Help: "The info of the last found staking transaction",
		},
		[]string{
			// the height of the block containing the staking tx
			"height",
			// the tx id of the staking tx
			"txid",
			// the public key of the staker
			"staker_pk",
			// the staking value
			"staking_value",
			// the time that the staked BTC is locked in terms of blocks
			"staking_time",
			// the public key of the delegated finality provider
			"finality_provider_pk",
		},
	)

	lastFoundUnbondingTx = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "si_last_found_unbonding_tx",
			Help: "The info of the last found unbonding transaction",
		},
		[]string{
			// the height of the block containing the unbonding tx
			"height",
			// the tx id of the unbonding tx
			"txid",
			// the tx id of the relevant staking tx
			"staking_txid",
		},
	)

	lastFoundWithdrawTxFromStaking = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "si_last_found_withdraw_tx_from_staking",
			Help: "The info of the last found withdraw transaction from staking",
		},
		[]string{
			// the height of the block containing the withdrawal tx
			"height",
			// the tx id of the withdrawal tx
			"txid",
			// the tx id of the relevant staking tx
			"staking_txid",
		},
	)

	lastFoundWithdrawTxFromUnbonding = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "si_last_found_withdraw_tx_from_unbonding",
			Help: "The info of the last found withdraw transaction from unbonding",
		},
		[]string{
			// the height of the block containing the withdrawal tx
			"height",
			// the tx id of the withdrawal tx
			"txid",
			// the tx id of the relevant unbonding tx
			"unbonding_txid",
			// the tx id of the relevant staking tx
			"staking_txid",
		},
	)

	totalStakingTxs = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_total_staking_txs",
			Help: "Total number of staking transactions",
		},
	)

	totalUnbondingTxs = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_total_unbonding_txs",
			Help: "Total number of unbonding transactions",
		},
	)

	totalWithdrawTxsFromStaking = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_total_withdraw_txs_from_staking",
			Help: "Total number of withdraw transactions from staking path",
		},
	)

	totalWithdrawTxsFromUnbonding = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_total_withdraw_txs_from_unbonding",
			Help: "Total number of withdraw transactions from unbonding path",
		},
	)

	/* alerts */

	failedProcessingStakingTxsCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_processing_staking_txs_counter",
			Help: "Total number of failed staking txs during processing",
		},
	)

	failedCheckingUnbondingTxsCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_checking_unbonding_txs_counter",
			Help: "Total number of failed unbonding txs during checking",
		},
	)

	failedProcessingUnbondingTxsCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_processing_unbonding_txs_counter",
			Help: "Total number of failed unbonding txs during processing",
		},
	)

	failedProcessingWithdrawTxsFromStakingCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_processing_withdraw_txs_from_staking_counter",
			Help: "Total number of failed withdraw txs from staking during processing",
		},
	)

	failedProcessingWithdrawTxsFromUnbondingCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_processing_withdraw_txs_from_unbonding_counter",
			Help: "Total number of failed withdraw txs from unbonding during processing",
		},
	)
)

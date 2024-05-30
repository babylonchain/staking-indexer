package indexer

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	/* statistics */

	startBtcHeight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "si_start_btc_height",
		Help: "The BTC height from which the indexer starts scanning",
	})

	lastProcessedBtcHeight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "si_last_processed_btc_height",
		Help: "Last processed BTC height",
	})

	lastCalculatedTvl = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "si_last_calculated_active_tvl",
			Help: "The value of the last calculated TVL in satoshis",
		},
	)

	lastFoundStakingTxHeight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "si_last_found_staking_tx_height",
			Help: "The inclusion height of the last found staking transaction",
		},
	)

	lastFoundUnbondingTxHeight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "si_last_found_unbonding_tx_height",
			Help: "The inclusion height of the last found unbonding transaction",
		},
	)

	lastFoundWithdrawTxFromStakingHeight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "si_last_found_withdraw_tx_from_staking_height",
			Help: "The inclusion height of the last found withdrawal transaction spending a previous staking transaction ",
		},
	)

	lastFoundWithdrawTxFromUnbondingHeight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "si_last_found_withdraw_tx_from_unbonding_height",
			Help: "The inclusion height of the last found withdrawal transaction spending a previous unbonding transaction",
		},
	)

	totalStakingTxs = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "si_total_staking_txs",
			Help: "Total number of staking transactions",
		},
		[]string{
			"tx_type",
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
			Help: "Total number of withdrawal transactions from the staking path",
		},
	)

	totalWithdrawTxsFromUnbonding = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_total_withdraw_txs_from_unbonding",
			Help: "Total number of withdrawal transactions from the unbonding path",
		},
	)

	/* alerts */

	failedProcessingStakingTxsCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_processing_staking_txs_counter",
			Help: "Total number of failures when processing valid staking transactions",
		},
	)

	failedProcessingUnconfirmedBlockCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_processing_unconfirmed_block_counter",
			Help: "Total number of failures when processing unconfirmed blocks",
		},
	)

	invalidTransactionsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "si_invalid_txs_counter",
			Help: "Total number of invalid transactions",
		},
		[]string{
			"tx_type",
		},
	)

	failedVerifyingUnbondingTxsCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_checking_unbonding_txs_counter",
			Help: "Total number of failures when verifying unbonding txs ",
		},
	)

	failedProcessingUnbondingTxsCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_processing_unbonding_txs_counter",
			Help: "Total number of failures when processing valid unbonding transactions",
		},
	)

	failedProcessingWithdrawTxsFromStakingCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_processing_withdraw_txs_from_staking_counter",
			Help: "Total number of failures when processing valid withdrawal transactions from staking ",
		},
	)

	failedProcessingWithdrawTxsFromUnbondingCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_processing_withdraw_txs_from_unbonding_counter",
			Help: "Total number of failures when processing valid withdrawal transactions from unbonding",
		},
	)
)

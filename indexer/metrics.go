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
			Help: "The info of the last found withdrawal transaction spending a previous staking transaction ",
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
			Help: "The info of the last found withdrawal transaction spending a previous unbonding transaction",
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

	lastCalculatedTvlInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "si_last_calculated_active_tvl",
			Help: "The info of the last calculated TVL",
		},
		[]string{
			// the BTC block height up to which the tvl is calculated
			"tip_height",
			// the confirmed BTC block height
			"confirmed_height",
			// the value of the confirmed tvl in satoshis
			"confirmed_tvl",
			// the value of the unconfirmed tvl in satoshis
			"unconfirmed_tvl",
			// the value of the total tvl in satoshis
			"total_tvl",
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

	invalidStakingTxsCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_invalid_staking_txs_counter",
			Help: "Total number of invalid transactions",
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

	invalidUnbondingTxsCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_invalid_unbonding_txs_counter",
			Help: "Total number of invalid transactions",
		},
	)
)

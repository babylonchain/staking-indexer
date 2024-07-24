package indexer

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Based on github.com/babylonchain/staking-indexer/indexer/metrics.go
var (
	lastFoundVaultTxHeight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "si_last_found_vault_tx_height",
			Help: "The inclusion height of the last found vault transaction",
		},
	)

	lastFoundBurningTxHeight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "si_last_found_burning_tx_height",
			Help: "The inclusion height of the last found burning transaction",
		},
	)

	lastFoundWithdrawTxFromVaultHeight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "si_last_found_withdraw_tx_from_vault_height",
			Help: "The inclusion height of the last found withdrawal transaction spending a previous vault transaction ",
		},
	)

	lastFoundWithdrawTxFromBurningHeight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "si_last_found_withdraw_tx_from_burning_height",
			Help: "The inclusion height of the last found withdrawal transaction spending a previous burning transaction",
		},
	)

	totalVaultTxs = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "si_total_vault_txs",
			Help: "Total number of vault transactions",
		},
		[]string{
			"tx_type",
		},
	)

	totalBurningTxs = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_total_burning_txs",
			Help: "Total number of burning transactions",
		},
	)

	totalWithdrawTxsFromVault = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_total_withdraw_txs_from_vault",
			Help: "Total number of withdrawal transactions from the vault path",
		},
	)

	totalWithdrawTxsFromBurning = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_total_withdraw_txs_from_burning",
			Help: "Total number of withdrawal transactions from the burning path",
		},
	)

	/* alerts */

	failedProcessingVaultTxsCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_processing_vault_txs_counter",
			Help: "Total number of failures when processing valid vault transactions",
		},
	)

	failedVerifyingBurningTxsCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_checking_burning_txs_counter",
			Help: "Total number of failures when verifying burning txs ",
		},
	)

	failedProcessingBurningTxsCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_processing_burning_txs_counter",
			Help: "Total number of failures when processing valid burning transactions",
		},
	)

	failedProcessingWithdrawTxsFromVaultCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_processing_withdraw_txs_from_vault_counter",
			Help: "Total number of failures when processing valid withdrawal transactions from vault ",
		},
	)

	failedProcessingWithdrawTxsFromBurningCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_failed_processing_withdraw_txs_from_burning_counter",
			Help: "Total number of failures when processing valid withdrawal transactions from burning",
		},
	)
)

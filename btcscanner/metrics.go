package btcscanner

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	majorReorgsCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "si_major_reorgs_counter",
			Help: "Total number of major reorgs happened",
		},
	)
)

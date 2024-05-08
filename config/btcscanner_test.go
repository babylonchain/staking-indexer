package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/babylonchain/staking-indexer/config"
)

func TestBTCScannerParams(t *testing.T) {
	// default BTC scanner config should be valid
	params := config.DefaultBTCScannerConfig()
	err := params.Validate()
	require.NoError(t, err)

	// test invalid polling-interval
	params.PollingInterval = -1
	err = params.Validate()
	require.ErrorContains(t, err, "polling-interval")
}

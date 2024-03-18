//go:build e2e
// +build e2e

package e2etest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	eventuallyWaitTimeOut = 10 * time.Second
	eventuallyPollTime    = 250 * time.Millisecond
)

func TestBTCScanner(t *testing.T) {
	n := 100
	tm := StartManagerWithNBlocks(t, n)
	defer tm.Stop()

	count, err := tm.BitcoindHandler.GetBlockCount()
	require.NoError(t, err)
	require.Equal(t, n, count)

	require.Eventually(t, func() bool {
		confirmedTip := tm.BS.ConfirmedTipBlock()
		return confirmedTip.Height == int32(n-int(tm.Config.BTCScannerConfig.ConfirmationDepth))
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

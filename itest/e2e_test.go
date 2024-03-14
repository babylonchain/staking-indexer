//go:build e2e
// +build e2e

package e2etest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBTCScanner(t *testing.T) {
	tm := StartManager(t)
	defer tm.Stop()

	numBlocks := 10
	res := tm.BitcoindHandler.GenerateBlocks(numBlocks)
	generatedBlocks := res.Blocks

	for i := 0; i < numBlocks-int(tm.Config.BTCScannerConfig.ConfirmationDepth); i++ {
		confirmedBlock := <-tm.ConfirmedBlocksChan
		require.Equal(t, generatedBlocks[0], confirmedBlock.BlockHash().String())
	}
}

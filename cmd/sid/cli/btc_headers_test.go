package cli_test

import (
	"math/rand"
	"testing"

	bbnbtclightclienttypes "github.com/babylonchain/babylon/x/btclightclient/types"
	"github.com/babylonchain/staking-indexer/btcclient"
	"github.com/babylonchain/staking-indexer/cmd/sid/cli"
	e2etest "github.com/babylonchain/staking-indexer/itest"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBtcHeaders(t *testing.T) {
	r := rand.New(rand.NewSource(10))
	blocksPerRetarget := 2016
	genState := bbnbtclightclienttypes.DefaultGenesis()

	initBlocksQnt := r.Intn(15) + blocksPerRetarget
	btcd, btcClient := StartBtcClientAndBtcHandler(t, initBlocksQnt)

	// from zero height
	infos, err := cli.BtcHeaderInfo(btcClient, 0, uint64(initBlocksQnt))
	require.NoError(t, err)
	require.Equal(t, len(infos), initBlocksQnt)

	// should be valid on genesis, start from zero height.
	genState.BtcHeaders = infos
	require.NoError(t, genState.Validate())

	generatedBlocksQnt := r.Intn(15) + 2
	btcd.GenerateBlocks(generatedBlocksQnt)
	totalBlks := initBlocksQnt + generatedBlocksQnt

	// check from height with interval
	fromBlockHeight := blocksPerRetarget - 1
	toBlockHeight := totalBlks - 2

	infos, err = cli.BtcHeaderInfo(btcClient, uint64(fromBlockHeight), uint64(toBlockHeight))
	require.NoError(t, err)
	require.Equal(t, len(infos), int(toBlockHeight-fromBlockHeight))

	// try to check if it is valid on genesis, should fail is not retarget block.
	genState.BtcHeaders = infos
	require.EqualError(t, genState.Validate(), "genesis block must be a difficulty adjustment block")

	// from retarget block
	infos, err = cli.BtcHeaderInfo(btcClient, uint64(blocksPerRetarget), uint64(totalBlks))
	require.NoError(t, err)
	require.Equal(t, len(infos), int(totalBlks-blocksPerRetarget))

	// check if it is valid on genesis
	genState.BtcHeaders = infos
	require.NoError(t, genState.Validate())
}

func StartBtcClientAndBtcHandler(t *testing.T, generateNBlocks int) (*e2etest.BitcoindTestHandler, *btcclient.BTCClient) {
	btcd := e2etest.NewBitcoindHandler(t)
	btcd.Start()
	_ = btcd.CreateWallet(e2etest.WalletName, e2etest.Passphrase)

	resp := btcd.GenerateBlocks(generateNBlocks)
	require.Equal(t, len(resp.Blocks), generateNBlocks)

	cfg := e2etest.DefaultStakingIndexerConfig(t.TempDir())
	btcClient, err := btcclient.NewBTCClient(
		cfg.BTCConfig,
		zap.NewNop(),
	)
	require.NoError(t, err)

	return btcd, btcClient
}

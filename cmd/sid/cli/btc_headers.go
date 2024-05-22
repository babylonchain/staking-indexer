package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	babylontypes "github.com/babylonchain/babylon/types"
	bbnbtclightclienttypes "github.com/babylonchain/babylon/x/btclightclient/types"
	"github.com/babylonchain/staking-indexer/btcclient"
	"github.com/babylonchain/staking-indexer/btcscanner"
	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/log"
	"github.com/babylonchain/staking-indexer/utils"
	"github.com/urfave/cli"
	"go.uber.org/zap"

	sdkmath "cosmossdk.io/math"
)

const (
	outputFileFlag        = "output"
	defaultOutputFileName = "btc-headers.json"
)

var BtcHeaderCommand = cli.Command{
	Name:        "btc-headers",
	Usage:       fmt.Sprintf("btc-headers [from] [to] [--%s=path/to/btc-headers.json]", outputFileFlag),
	Description: "Output a range of BTC headers into a JSON file.",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "The path to the staking indexer home directory",
			Value: config.DefaultHomeDir,
		},
		cli.StringFlag{
			Name:  outputFileFlag,
			Usage: "The path to the output file",
			Value: filepath.Join(config.DefaultHomeDir, defaultOutputFileName),
		},
	},
	Action: btcHeaders,
}

func btcHeaders(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 2 {
		return fmt.Errorf("not enough params, please specify [from] and [to]")
	}

	fromStr, toStr := args[0], args[1]
	fromBlock, err := strconv.ParseUint(fromStr, 10, 64)
	if err != nil {
		return fmt.Errorf("unable to parse %s: %w", fromStr, err)
	}

	toBlock, err := strconv.ParseUint(toStr, 10, 64)
	if err != nil {
		return fmt.Errorf("unable to parse %s: %w", toStr, err)
	}

	if fromBlock > toBlock {
		return fmt.Errorf("the [from] %d should not be greater than the [to] %d", fromBlock, toBlock)
	}

	homePath, err := filepath.Abs(ctx.String(homeFlag))
	if err != nil {
		return err
	}
	homePath = utils.CleanAndExpandPath(homePath)

	cfg, err := config.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger, err := log.NewRootLoggerWithFile(config.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize the logger: %w", err)
	}

	btcClient, err := btcclient.NewBTCClient(
		cfg.BTCConfig,
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize the BTC client: %w", err)
	}

	btcHeaders, err := BtcHeaderInfoList(btcClient, fromBlock, toBlock)
	if err != nil {
		return fmt.Errorf("failed to get BTC headers: %w", err)
	}

	genState := bbnbtclightclienttypes.GenesisState{
		BtcHeaders: btcHeaders,
	}

	bz, err := json.MarshalIndent(genState, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to generate json to set to output file %+v: %w", genState, err)
	}

	outputFilePath := ctx.String(outputFileFlag)
	if err := os.WriteFile(outputFilePath, bz, 0644); err != nil {
		return fmt.Errorf("failed to write to output file %s: %w", outputFilePath, err)
	}

	logger.Info(
		"Successfully wrote btc headers to file",
		zap.Uint64("fromBlock", fromBlock),
		zap.Uint64("toBlock", toBlock),
		zap.String("outputFile", outputFilePath),
	)
	return nil
}

// BtcHeaderInfoList queries the btc client for (fromBlk ~ toBlk) BTC blocks, converting to BTCHeaderInfo.
func BtcHeaderInfoList(btcClient btcscanner.Client, fromBlk, toBlk uint64) ([]*bbnbtclightclienttypes.BTCHeaderInfo, error) {
	btcHeaders := make([]*bbnbtclightclienttypes.BTCHeaderInfo, 0, toBlk-fromBlk+1)
	var currenWork = sdkmath.ZeroUint()

	for blkHeight := fromBlk; blkHeight <= toBlk; blkHeight++ {
		idxBlock, err := btcClient.GetBlockByHeight(blkHeight)
		if err != nil {
			return nil, fmt.Errorf("failed to get block height %d from BTC client: %w", blkHeight, err)
		}
		blkHeader := idxBlock.Header

		headerWork := bbnbtclightclienttypes.CalcHeaderWork(blkHeader)
		currenWork = bbnbtclightclienttypes.CumulativeWork(headerWork, currenWork)

		headerBytes := babylontypes.NewBTCHeaderBytesFromBlockHeader(blkHeader)

		bbnBtcHeaderInfo := bbnbtclightclienttypes.NewBTCHeaderInfo(&headerBytes, headerBytes.Hash(), blkHeight, &currenWork)
		btcHeaders = append(btcHeaders, bbnBtcHeaderInfo)
	}
	return btcHeaders, nil
}

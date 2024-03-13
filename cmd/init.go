package main

import (
	"fmt"
	"path/filepath"

	"github.com/jessevdk/go-flags"
	"github.com/urfave/cli"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/utils"
)

const forceFlag = "force"

var initCommand = cli.Command{
	Name:  "init",
	Usage: "Initialize the staking indexer home directory.",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "Path to where the home directory will be initialized",
			Value: config.DefaultHomeDir,
		},
		cli.BoolFlag{
			Name:     forceFlag,
			Usage:    "Override existing configuration",
			Required: false,
		},
	},
	Action: initHome,
}

func initHome(c *cli.Context) error {
	homePath, err := filepath.Abs(c.String(homeFlag))
	if err != nil {
		return err
	}
	// Create home directory
	homePath = utils.CleanAndExpandPath(homePath)
	force := c.Bool(forceFlag)

	if utils.FileExists(homePath) && !force {
		return fmt.Errorf("home path %s already exists", homePath)
	}

	if err := utils.MakeDirectory(homePath); err != nil {
		return err
	}
	// Create log directory
	logDir := config.LogDir(homePath)
	if err := utils.MakeDirectory(logDir); err != nil {
		return err
	}
	// Create data directory
	dataDir := config.DataDir(homePath)
	if err := utils.MakeDirectory(dataDir); err != nil {
		return err
	}

	defaultConfig := config.DefaultConfig()
	fileParser := flags.NewParser(&defaultConfig, flags.Default)

	return flags.NewIniParser(fileParser).WriteFile(config.ConfigFile(homePath), flags.IniIncludeComments|flags.IniIncludeDefaults)
}

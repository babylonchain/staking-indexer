package config

import (
	"fmt"
	"path/filepath"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/jessevdk/go-flags"

	"github.com/babylonchain/staking-indexer/utils"
)

const (
	defaultLogLevel       = "info"
	defaultLogDirname     = "logs"
	defaultLogFilename    = "sid.log"
	defaultConfigFileName = "sid.conf"
	defaultBitcoinNetwork = "signet"
	defaultDataDirname    = "data"
)

var (
	//   C:\Users\<username>\AppData\Local\ on Windows
	//   ~/.fpd on Linux
	//   ~/Users/<username>/Library/Application Support/Sid on MacOS
	DefaultHomeDir = btcutil.AppDataDir("sid", false)
)

// Config is the main config for the fpd cli command
type Config struct {
	LogLevel string `long:"loglevel" description:"Logging level for all subsystems" choice:"trace" choice:"debug" choice:"info" choice:"warn" choice:"error" choice:"fatal"`

	BitcoinNetwork string `long:"bitcoinnetwork" description:"Bitcoin network to run on" choise:"mainnet" choice:"regtest" choice:"testnet" choice:"simnet" choice:"signet"`

	BTCNetParams chaincfg.Params
}

func DefaultConfigWithHome(homePath string) Config {
	cfg := Config{
		LogLevel:       defaultLogLevel,
		BitcoinNetwork: defaultBitcoinNetwork,
	}

	if err := cfg.Validate(); err != nil {
		panic(err)
	}

	return cfg
}

func DefaultConfig() Config {
	return DefaultConfigWithHome(DefaultHomeDir)
}

func ConfigFile(homePath string) string {
	return filepath.Join(homePath, defaultConfigFileName)
}

func LogDir(homePath string) string {
	return filepath.Join(homePath, defaultLogDirname)
}

func LogFile(homePath string) string {
	return filepath.Join(LogDir(homePath), defaultLogFilename)
}

func DataDir(homePath string) string {
	return filepath.Join(homePath, defaultDataDirname)
}

// LoadConfig initializes and parses the config using a config file and command
// line options.
//
// The configuration proceeds as follows:
//  1. Start with a default config with sane settings
//  2. Pre-parse the command line to check for an alternative config file
//  3. Load configuration file overwriting defaults with any specified options
//  4. Parse CLI options and overwrite/add any specified options
func LoadConfig(homePath string) (*Config, error) {
	// The home directory is required to have a configuration file with a specific name
	// under it.
	cfgFile := ConfigFile(homePath)
	if !utils.FileExists(cfgFile) {
		return nil, fmt.Errorf("specified config file does "+
			"not exist in %s", cfgFile)
	}

	// Next, load any additional configuration options from the file.
	var cfg Config
	fileParser := flags.NewParser(&cfg, flags.Default)
	err := flags.NewIniParser(fileParser).ParseFile(cfgFile)
	if err != nil {
		return nil, err
	}

	// Make sure everything we just loaded makes sense.
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate checks the given configuration to be sane. This makes sure no
// illegal values or combination of values are set. All file system paths are
// normalized. The cleaned up config is returned on success.
func (cfg *Config) Validate() error {
	// Multiple networks can't be selected simultaneously.  Count number of
	// network flags passed; assign active network params
	// while we're at it.
	switch cfg.BitcoinNetwork {
	case "mainnet":
		cfg.BTCNetParams = chaincfg.MainNetParams
	case "testnet":
		cfg.BTCNetParams = chaincfg.TestNet3Params
	case "regtest":
		cfg.BTCNetParams = chaincfg.RegressionNetParams
	case "simnet":
		cfg.BTCNetParams = chaincfg.SimNetParams
	case "signet":
		cfg.BTCNetParams = chaincfg.SigNetParams
	default:
		return fmt.Errorf("invalid network: %v", cfg.BitcoinNetwork)
	}

	// All good, return the sanitized result.
	return nil
}

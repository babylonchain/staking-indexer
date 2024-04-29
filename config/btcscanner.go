package config

import (
	"fmt"
	"time"
)

const (
	defaultPollingInterval   = 1 * time.Second
	defaultConfirmationDepth = 10
	defaultBaseHeight        = 1
)

type BTCScannerConfig struct {
	BaseHeight        uint64        `long:"baseheight" description:"the base height from which the polling starts to avoid unnecessary polling"`
	PollingInterval   time.Duration `long:"pollinginterval" description:"the time interval between each polling of new confirmed blocks"`
	ConfirmationDepth uint64        `long:"confirmationdepth" description:"the confirmation depth to consider a BTC block as confirmed"`
}

func (cfg *BTCScannerConfig) Validate() error {
	if cfg.BaseHeight == 0 {
		return fmt.Errorf("base-height %d must be positive", cfg.BaseHeight)
	}

	if cfg.ConfirmationDepth == 0 {
		return fmt.Errorf("confirmation-depth %d must be positive", cfg.ConfirmationDepth)
	}

	if cfg.PollingInterval <= 0 {
		return fmt.Errorf("polling-interval %v must be positive", cfg.PollingInterval)
	}
	return nil
}

func DefaultBTCScannerConfig() *BTCScannerConfig {
	return &BTCScannerConfig{
		BaseHeight:        defaultBaseHeight,
		PollingInterval:   defaultPollingInterval,
		ConfirmationDepth: defaultConfirmationDepth,
	}
}

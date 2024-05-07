package config

import (
	"fmt"
	"time"
)

const (
	defaultPollingInterval = 1 * time.Minute
	defaultBaseHeight      = 1
)

type BTCScannerConfig struct {
	BaseHeight      uint64        `long:"baseheight" description:"the base height before which no staking transactions exist"`
	PollingInterval time.Duration `long:"pollinginterval" description:"the time interval between each polling of new confirmed blocks"`
}

func (cfg *BTCScannerConfig) Validate() error {
	if cfg.BaseHeight == 0 {
		return fmt.Errorf("base-height %d must be positive", cfg.BaseHeight)
	}

	if cfg.PollingInterval <= 0 {
		return fmt.Errorf("polling-interval %v must be positive", cfg.PollingInterval)
	}
	return nil
}

func DefaultBTCScannerConfig() *BTCScannerConfig {
	return &BTCScannerConfig{
		BaseHeight:      defaultBaseHeight,
		PollingInterval: defaultPollingInterval,
	}
}

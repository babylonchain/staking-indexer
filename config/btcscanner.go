package config

import (
	"fmt"
	"time"
)

const (
	defaultPollingInterval   = 1 * time.Second
	defaultConfirmationDepth = 10
)

type BTCScannerConfig struct {
	PollingInterval   time.Duration `long:"pollinginterval" description:"the time interval between each polling of new confirmed blocks"`
	ConfirmationDepth uint64        `long:"confirmationdepth" description:"the confirmation depth to consider a BTC block as confirmed"`
}

func (cfg *BTCScannerConfig) Validate() error {
	if cfg.ConfirmationDepth < defaultConfirmationDepth {
		return fmt.Errorf("btc-confirmation-depth should not be less than %d", defaultConfirmationDepth)
	}
	return nil
}

func DefaultBTCScannerConfig() *BTCScannerConfig {
	return &BTCScannerConfig{
		PollingInterval:   defaultPollingInterval,
		ConfirmationDepth: defaultConfirmationDepth,
	}
}

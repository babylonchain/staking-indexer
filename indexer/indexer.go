package indexer

import (
	"sync"

	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
)

type StakingIndexer struct {
	startOnce sync.Once
	stopOnce  sync.Once

	cfg    *config.Config
	logger *zap.Logger

	wg   sync.WaitGroup
	quit chan struct{}
}

func NewStakingIndexer(
	cfg *config.Config,
	logger *zap.Logger,
) (*StakingIndexer, error) {

	return &StakingIndexer{
		cfg:    cfg,
		logger: logger,
		quit:   make(chan struct{}),
	}, nil
}

// Start starts the staking indexer core
func (si *StakingIndexer) Start() error {
	var startErr error
	si.startOnce.Do(func() {
		si.logger.Info("Starting Staking Indexer App")
		si.logger.Info("Staking Indexer App is successfully started!")
	})

	return startErr
}

func (si *StakingIndexer) Stop() error {
	var stopErr error
	si.stopOnce.Do(func() {
		si.logger.Info("Stopping Staking Indexer App")

		close(si.quit)
		si.wg.Wait()

		si.logger.Info("Staking Indexer App is successfully stopped!")

	})
	return stopErr
}

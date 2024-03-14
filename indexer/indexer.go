package indexer

import (
	"sync"

	vtypes "github.com/babylonchain/vigilante/types"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
)

type StakingIndexer struct {
	startOnce sync.Once
	stopOnce  sync.Once

	cfg    *config.Config
	logger *zap.Logger

	confirmedBlocksChan chan *vtypes.IndexedBlock

	wg   sync.WaitGroup
	quit chan struct{}
}

func NewStakingIndexer(
	cfg *config.Config,
	logger *zap.Logger,
	confirmedBlocksChan chan *vtypes.IndexedBlock,
) (*StakingIndexer, error) {

	return &StakingIndexer{
		cfg:                 cfg,
		logger:              logger.With(zap.String("module", "staking indexer")),
		confirmedBlocksChan: confirmedBlocksChan,
		quit:                make(chan struct{}),
	}, nil
}

// Start starts the staking indexer core
func (si *StakingIndexer) Start() error {
	var startErr error
	si.startOnce.Do(func() {
		si.logger.Info("Starting Staking Indexer App")

		si.wg.Add(1)
		go si.confirmedBlocksLoop()

		si.logger.Info("Staking Indexer App is successfully started!")
	})

	return startErr
}

func (si *StakingIndexer) confirmedBlocksLoop() {
	defer si.wg.Done()

	for {
		select {
		case block := <-si.confirmedBlocksChan:
			si.logger.Info("received confirmed block",
				zap.Int32("height", block.Height))
			// TODO: process confirmed block
		case <-si.quit:
			si.logger.Info("closing the confirmed blocks loop")
			return
		}
	}
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

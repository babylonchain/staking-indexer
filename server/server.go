package service

import (
	"fmt"
	"sync/atomic"

	"github.com/lightningnetwork/lnd/signal"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/btcscanner"
	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/indexer"
)

// Server is the main daemon construct for the staking indexer service. It handles
// spinning up the database, the indexer application and any other components
type Server struct {
	started int32

	scanner *btcscanner.BtcScanner
	si      *indexer.StakingIndexer

	cfg    *config.Config
	logger *zap.Logger

	interceptor signal.Interceptor
}

// NewStakingIndexerServer creates a new server with the given config.
func NewStakingIndexerServer(cfg *config.Config, scanner *btcscanner.BtcScanner, si *indexer.StakingIndexer, l *zap.Logger, sig signal.Interceptor) *Server {
	return &Server{
		cfg:         cfg,
		scanner:     scanner,
		si:          si,
		logger:      l,
		interceptor: sig,
	}
}

// RunUntilShutdown runs the main EOTS manager server loop until a signal is
// received to shut down the process.
func (s *Server) RunUntilShutdown() error {
	if atomic.AddInt32(&s.started, 1) != 1 {
		return nil
	}

	defer func() {
		s.logger.Info("Shutdown complete")
	}()

	s.logger.Info("starting the BTC scanner...")
	if err := s.scanner.Start(); err != nil {
		return fmt.Errorf("failed to start the BTC scanner: %w", err)
	}
	s.logger.Info("the BTC scanner is successfully started")
	defer func() {
		s.logger.Info("stopping the BTC scanner...")
		if err := s.scanner.Stop(); err != nil {
			s.logger.Error("failed to stop the BTC scanner")
		}
		s.logger.Info("the BTC scanner is successfully stopped")
	}()

	s.logger.Info("starting the staking indexer app")
	if err := s.si.Start(); err != nil {
		return fmt.Errorf("failed to start the staking indexer app: %w", err)
	}
	s.logger.Info("the staking indexer app is successfully started")
	defer func() {
		s.logger.Info("stopping the staking indexer app...")
		if err := s.si.Stop(); err != nil {
			s.logger.Error("failed to stop the staking indexer app")
		}
		s.logger.Info("the staking indexer app is successfully stopped")
	}()

	s.logger.Info("Staking Indexer service is fully active!")
	// Wait for shutdown signal from either a graceful server stop or from
	// the interrupt handler.
	<-s.interceptor.ShutdownChannel()

	return nil
}

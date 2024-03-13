package service

import (
	"fmt"
	"sync/atomic"

	"github.com/lightningnetwork/lnd/signal"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/indexer"
)

// Server is the main daemon construct for the staking indexer service. It handles
// spinning up the database, the indexer application and any other components
type Server struct {
	started int32

	si *indexer.StakingIndexer

	cfg    *config.Config
	logger *zap.Logger

	interceptor signal.Interceptor
}

// NewStakingIndexerServer creates a new server with the given config.
func NewStakingIndexerServer(cfg *config.Config, si *indexer.StakingIndexer, l *zap.Logger, sig signal.Interceptor) *Server {
	return &Server{
		cfg:         cfg,
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

	if err := s.si.Start(); err != nil {
		return fmt.Errorf("failed to start the staking indexer app: %w", err)
	}
	defer s.si.Stop()

	s.logger.Info("Staking Indexer service is fully active!")

	// Wait for shutdown signal from either a graceful server stop or from
	// the interrupt handler.
	<-s.interceptor.ShutdownChannel()

	return nil
}

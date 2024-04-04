package server

import (
	"fmt"
	"sync/atomic"

	notifier "github.com/lightningnetwork/lnd/chainntnfs"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/signal"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/consumer"
	"github.com/babylonchain/staking-indexer/indexer"
)

// Server is the main daemon construct for the staking indexer service. It handles
// spinning up the database, the indexer application and any other components
type Server struct {
	started int32

	si          *indexer.StakingIndexer
	btcNotifier notifier.ChainNotifier
	ec          consumer.EventConsumer

	db kvdb.Backend

	cfg    *config.Config
	logger *zap.Logger

	interceptor signal.Interceptor
}

// NewStakingIndexerServer creates a new server with the given config.
func NewStakingIndexerServer(
	cfg *config.Config,
	ec consumer.EventConsumer,
	db kvdb.Backend,
	btcNotifier notifier.ChainNotifier,
	si *indexer.StakingIndexer,
	l *zap.Logger,
	sig signal.Interceptor,
) *Server {
	return &Server{
		cfg:         cfg,
		si:          si,
		ec:          ec,
		db:          db,
		btcNotifier: btcNotifier,
		logger:      l,
		interceptor: sig,
	}
}

// RunUntilShutdown runs the main EOTS manager server loop until a signal is
// received to shut down the process.
func (s *Server) RunUntilShutdown(startHeight uint64) error {
	if atomic.AddInt32(&s.started, 1) != 1 {
		return nil
	}

	defer func() {
		s.logger.Info("Shutdown complete")
	}()

	defer func() {
		s.logger.Info("Closing database...")
		s.db.Close()
		s.logger.Info("Database closed")
	}()

	if err := s.btcNotifier.Start(); err != nil {
		return fmt.Errorf("failed to start the BTC notifier: %w", err)
	}
	defer func() {
		if err := s.btcNotifier.Stop(); err != nil {
			s.logger.Error("failed to stop the BTC notifier", zap.Error(err))
		}
	}()

	if err := s.ec.Start(); err != nil {
		return fmt.Errorf("failed to start the event consumer: %w", err)
	}
	defer func() {
		if err := s.ec.Stop(); err != nil {
			s.logger.Error("failed to stop the event consumer", zap.Error(err))
		}
	}()

	if err := s.si.Start(startHeight); err != nil {
		return fmt.Errorf("failed to start the staking indexer app: %w", err)
	}
	defer func() {
		if err := s.si.Stop(); err != nil {
			s.logger.Error("failed to stop the staking indexer app", zap.Error(err))
		}
	}()

	s.logger.Info("Staking Indexer service is fully active!")
	// Wait for shutdown signal from either a graceful server stop or from
	// the interrupt handler.
	<-s.interceptor.ShutdownChannel()

	return nil
}

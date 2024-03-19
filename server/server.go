package server

import (
	"fmt"
	"sync/atomic"

	notifier "github.com/lightningnetwork/lnd/chainntnfs"
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

	scanner     *btcscanner.BtcScanner
	si          *indexer.StakingIndexer
	btcNotifier notifier.ChainNotifier

	cfg    *config.Config
	logger *zap.Logger

	interceptor signal.Interceptor
}

// NewStakingIndexerServer creates a new server with the given config.
func NewStakingIndexerServer(cfg *config.Config, btcNotifier notifier.ChainNotifier, scanner *btcscanner.BtcScanner, si *indexer.StakingIndexer, l *zap.Logger, sig signal.Interceptor) *Server {
	return &Server{
		cfg:         cfg,
		scanner:     scanner,
		si:          si,
		btcNotifier: btcNotifier,
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

	if err := s.btcNotifier.Start(); err != nil {
		return fmt.Errorf("failed to start the BTC notifier: %w", err)
	}
	defer func() {
		if err := s.btcNotifier.Stop(); err != nil {
			s.logger.Error("failed to stop the BTC notifier")
		}
	}()

	if err := s.si.Start(); err != nil {
		return fmt.Errorf("failed to start the staking indexer app: %w", err)
	}
	defer func() {
		if err := s.si.Stop(); err != nil {
			s.logger.Error("failed to stop the staking indexer app")
		}
	}()

	if err := s.scanner.Start(); err != nil {
		return fmt.Errorf("failed to start the BTC scanner: %w", err)
	}
	defer func() {
		if err := s.scanner.Stop(); err != nil {
			s.logger.Error("failed to stop the BTC scanner")
		}
	}()

	s.logger.Info("Staking Indexer service is fully active!")
	// Wait for shutdown signal from either a graceful server stop or from
	// the interrupt handler.
	<-s.interceptor.ShutdownChannel()

	return nil
}

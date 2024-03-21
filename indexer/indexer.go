package indexer

import (
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/babylonchain/babylon/btcstaking"
	vtypes "github.com/babylonchain/vigilante/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/kvdb"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/consumer"
	"github.com/babylonchain/staking-indexer/indexerstore"
	"github.com/babylonchain/staking-indexer/types"
)

type StakingIndexer struct {
	startOnce sync.Once
	stopOnce  sync.Once

	consumer consumer.EventConsumer
	params   *types.Params

	cfg    *config.Config
	logger *zap.Logger

	is *indexerstore.IndexerStore

	confirmedBlocksChan chan *vtypes.IndexedBlock
	stakingEvenChan     chan *types.ActiveStakingEvent

	wg   sync.WaitGroup
	quit chan struct{}
}

func NewStakingIndexer(
	cfg *config.Config,
	logger *zap.Logger,
	consumer consumer.EventConsumer,
	db kvdb.Backend,
	params *types.Params,
	confirmedBlocksChan chan *vtypes.IndexedBlock,
) (*StakingIndexer, error) {

	is, err := indexerstore.NewIndexerStore(db)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate staking indexer store: %w", err)
	}

	return &StakingIndexer{
		cfg:                 cfg,
		logger:              logger.With(zap.String("module", "staking indexer")),
		consumer:            consumer,
		is:                  is,
		params:              params,
		confirmedBlocksChan: confirmedBlocksChan,
		stakingEvenChan:     make(chan *types.ActiveStakingEvent),
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
			b := block
			si.logger.Info("received confirmed block",
				zap.Int32("height", block.Height))
			if err := si.handleConfirmedBlock(b); err != nil {
				// this indicates systematic failure
				si.logger.Fatal("failed to handle block", zap.Error(err))
			}
		case <-si.quit:
			si.logger.Info("closing the confirmed blocks loop")
			return
		}
	}
}

// handleConfirmedBlock iterates all the tx set in the block and
// parse staking tx data if there are any
func (si *StakingIndexer) handleConfirmedBlock(b *vtypes.IndexedBlock) error {
	for _, tx := range b.Txs {
		msgTx := tx.MsgTx()

		stakingData, err := si.tryParseStakingTx(msgTx)
		if err == nil {
			si.logger.Info("found a staking tx",
				zap.Int32("height", b.Height),
				zap.String("tx_hash", msgTx.TxHash().String()),
				zap.String("staker_pk", hex.EncodeToString(stakingData.OpReturnData.StakerPublicKey.Marshall())),
				zap.Int64("staking_value", msgTx.TxOut[stakingData.StakingOutputIdx].Value),
				zap.Uint16("staking_time", stakingData.OpReturnData.StakingTime),
			)

			if err := si.processStakingTx(msgTx, stakingData, uint64(b.Height)); err != nil {
				return err
			}

			// TODO: add monitor for withdrawal tx

			continue
		}

		// TODO: try to parse unbonding tx from here
	}

	return nil
}

func (si *StakingIndexer) processStakingTx(tx *wire.MsgTx, stakingData *btcstaking.ParsedV0StakingTx, height uint64) error {

	stakingEvent := types.StakingDataToEvent(stakingData, tx.TxHash(), height)

	if err := si.consumer.PushStakingEvent(stakingEvent); err != nil {
		return fmt.Errorf("failed to push the staking event to the consumer: %w", err)
	}

	si.logger.Info("successfully pushing the staking event",
		zap.String("tx_hash", tx.TxHash().String()))

	if err := si.is.AddStakingTransaction(
		tx,
		uint32(stakingData.StakingOutputIdx),
		height,
		stakingData.OpReturnData.StakerPublicKey.PubKey,
		uint32(stakingData.OpReturnData.StakingTime),
		[]*btcec.PublicKey{stakingData.OpReturnData.FinalityProviderPublicKey.PubKey},
	); err != nil {
		return fmt.Errorf("failed to add the staking tx to store: %w", err)
	}

	si.logger.Info("successfully saving the staking tx",
		zap.String("tx_hash", tx.TxHash().String()))

	return nil
}

func (si *StakingIndexer) tryParseStakingTx(tx *wire.MsgTx) (*btcstaking.ParsedV0StakingTx, error) {
	possible := btcstaking.IsPossibleV0StakingTx(tx, si.params.MagicBytes)
	if !possible {
		return nil, fmt.Errorf("not staking tx")
	}

	parsedData, err := btcstaking.ParseV0StakingTx(
		tx,
		si.params.MagicBytes,
		si.params.CovenantPks,
		si.params.CovenantQuorum,
		&si.cfg.BTCNetParams)
	if err != nil {
		return nil, fmt.Errorf("not staking tx")
	}

	return parsedData, nil
}

func (si *StakingIndexer) StakingEventChan() chan *types.ActiveStakingEvent {
	return si.stakingEvenChan
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

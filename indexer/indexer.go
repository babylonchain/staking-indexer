package indexer

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/babylonchain/babylon/btcstaking"
	queuecli "github.com/babylonchain/staking-queue-client/client"
	vtypes "github.com/babylonchain/vigilante/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
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
// parse staking tx, unbonding tx, and withdrawal tx if there are any
func (si *StakingIndexer) handleConfirmedBlock(b *vtypes.IndexedBlock) error {
	for _, tx := range b.Txs {
		msgTx := tx.MsgTx()

		// 1. try to parse staking tx
		stakingData, err := si.tryParseStakingTx(msgTx)
		if err == nil {
			if err := si.processStakingTx(msgTx, stakingData, uint64(b.Height), b.Header.Timestamp); err != nil {
				if !errors.Is(err, indexerstore.ErrDuplicateTransaction) {
					return err
				}
				// we don't consider duplicate error critical as it can happen
				// when the indexer restarts
				si.logger.Warn("found a duplicate tx",
					zap.String("tx_hash", msgTx.TxHash().String()))
				continue
			}

			// should not use *continue* here as a special case is
			// the tx could be a staking tx as well as a withdrawal
			// tx that spends the previous staking tx
		}

		// 2. not a staking tx, check whether it spends a stored staking tx
		stakingTx, spentInputIdx := si.getSpentStakingTx(msgTx)
		if spentInputIdx >= 0 {
			stakingTxHash := stakingTx.Tx.TxHash()
			// 3. is a spending tx, check whether it is an unbonding tx
			isUnbonding, err := si.IsUnbondingTx(msgTx, stakingTx)
			if err != nil {
				return err
			}
			if !isUnbonding {
				// 4. not a unbongidng tx, so this is a withdraw tx from the staking
				if err := si.processWithdrawTx(msgTx, &stakingTxHash, nil); err != nil {
					return err
				}
				continue
			}

			// 5. this is an unbonding tx
			if err := si.processUnbondingTx(msgTx, &stakingTxHash, uint64(b.Height), b.Header.Timestamp); err != nil {
				if !errors.Is(err, indexerstore.ErrDuplicateTransaction) {
					return err
				}
				// we don't consider duplicate error critical as it can happen
				// when the indexer restarts
				si.logger.Warn("found a duplicate tx",
					zap.String("tx_hash", msgTx.TxHash().String()))
			}
			continue
		}

		// 6. it does not spend staking tx, check whether it spends stored
		// unbonding tx
		unbondingTx, spentInputIdx := si.getSpentUnbondingTx(msgTx)
		if spentInputIdx >= 0 {
			// 7. this is a withdraw tx from the unbonding
			unbondingTxHash := unbondingTx.Tx.TxHash()
			if err := si.processWithdrawTx(msgTx, unbondingTx.StakingTxHash, &unbondingTxHash); err != nil {
				return err
			}
		}
	}

	return nil
}

// getSpentStakingTx checks if the given tx spends any of the stored staking tx
// if so, it returns the found staking tx and the spent staking input index,
// otherwise, it returns nil and -1
func (si *StakingIndexer) getSpentStakingTx(tx *wire.MsgTx) (*indexerstore.StoredStakingTransaction, int) {
	for i, txIn := range tx.TxIn {
		maybeStakingTxHash := txIn.PreviousOutPoint.Hash
		stakingTx, err := si.GetStakingTxByHash(&maybeStakingTxHash)
		if err != nil {
			continue
		}

		// this ensures the spending tx spends the correct staking output
		if txIn.PreviousOutPoint.Index != stakingTx.StakingOutputIdx {
			continue
		}

		return stakingTx, i
	}

	return nil, -1
}

// getSpentStakingTx checks if the given tx spends any of the stored staking tx
// if so, it returns the found staking tx and the spent staking input index,
// otherwise, it returns nil and -1
func (si *StakingIndexer) getSpentUnbondingTx(tx *wire.MsgTx) (*indexerstore.StoredUnbondingTransaction, int) {
	for i, txIn := range tx.TxIn {
		maybeUnbondingTxHash := txIn.PreviousOutPoint.Hash
		unbondingTx, err := si.GetUnbondingTxByHash(&maybeUnbondingTxHash)
		if err != nil {
			continue
		}

		return unbondingTx, i
	}

	return nil, -1
}

// IsUnbondingTx tries to identify a tx is an unbonding tx
// if provided tx is not unbonding tx it returns error.
func (si *StakingIndexer) IsUnbondingTx(tx *wire.MsgTx, stakingTx *indexerstore.StoredStakingTransaction) (bool, error) {
	// 1. the tx must have exactly one input and output
	if len(tx.TxIn) != 1 {
		return false, nil
	}
	if len(tx.TxOut) != 1 {
		return false, nil
	}

	// 2. the tx must spend the staking output
	stakingTxHash := stakingTx.Tx.TxHash()
	if !tx.TxIn[0].PreviousOutPoint.Hash.IsEqual(&stakingTxHash) {
		return false, nil
	}
	if tx.TxIn[0].PreviousOutPoint.Index != stakingTx.StakingOutputIdx {
		return false, nil
	}

	// 3. the script of the unbonding output must be as expected
	// re-build unbonding output from params, unbonding amount could be 0 as it will
	// not be considered as part of PkScript
	expectedUnbondingOutput, err := btcstaking.BuildUnbondingInfo(
		stakingTx.StakerPk,
		[]*btcec.PublicKey{stakingTx.FinalityProviderPk},
		si.params.CovenantPks,
		si.params.CovenantQuorum,
		si.params.UnbondingTime,
		0,
		&si.cfg.BTCNetParams,
	)
	if err != nil {
		return false, fmt.Errorf("failed to rebuild unbonding output: %w", err)
	}
	if !bytes.Equal(tx.TxOut[0].PkScript, expectedUnbondingOutput.UnbondingOutput.PkScript) {
		return false, nil
	}

	return true, nil
}

func (si *StakingIndexer) processStakingTx(
	tx *wire.MsgTx,
	stakingData *btcstaking.ParsedV0StakingTx,
	height uint64, timestamp time.Time,
) error {

	si.logger.Info("found a staking tx",
		zap.Uint64("height", height),
		zap.String("tx_hash", tx.TxHash().String()),
	)

	txHex, err := getTxHex(tx)
	if err != nil {
		return err
	}

	stakingEvent := queuecli.NewActiveStakingEvent(
		tx.TxHash().String(),
		hex.EncodeToString(stakingData.OpReturnData.StakerPublicKey.Marshall()),
		hex.EncodeToString(stakingData.OpReturnData.FinalityProviderPublicKey.Marshall()),
		uint64(stakingData.StakingOutput.Value),
		height,
		timestamp.String(),
		uint64(stakingData.OpReturnData.StakingTime),
		uint64(stakingData.StakingOutputIdx),
		txHex,
	)

	// push the events first with the assumption that the consumer can handle duplicate events
	if err := si.consumer.PushStakingEvent(&stakingEvent); err != nil {
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
		stakingData.OpReturnData.FinalityProviderPublicKey.PubKey,
	); err != nil {
		return fmt.Errorf("failed to add the staking tx to store: %w", err)
	}

	si.logger.Info("successfully saving the staking tx",
		zap.String("tx_hash", tx.TxHash().String()))

	return nil
}

func (si *StakingIndexer) processUnbondingTx(
	tx *wire.MsgTx,
	stakingTxHash *chainhash.Hash,
	height uint64, timestamp time.Time,
) error {

	si.logger.Info("found an unbonding tx",
		zap.Uint64("height", height),
		zap.String("tx_hash", tx.TxHash().String()),
		zap.String("staking_tx_hash", stakingTxHash.String()),
	)

	txHex, err := getTxHex(tx)
	if err != nil {
		return err
	}

	unbondingEvent := queuecli.NewUnbondingStakingEvent(
		stakingTxHash.String(),
		height,
		strconv.FormatInt(timestamp.Unix(), 10),
		uint64(si.params.UnbondingTime),
		// valid unbonding tx always has one output
		0,
		txHex,
		tx.TxHash().String(),
	)

	if err := si.consumer.PushUnbondingEvent(&unbondingEvent); err != nil {
		return fmt.Errorf("failed to push the unbonding event to the consumer: %w", err)
	}

	si.logger.Info("successfully pushing the unbonding event",
		zap.String("tx_hash", tx.TxHash().String()))

	if err := si.is.AddUnbondingTransaction(
		tx,
		stakingTxHash,
	); err != nil {
		return fmt.Errorf("failed to add the unbonding tx to store: %w", err)
	}

	si.logger.Info("successfully saving the unbonding tx",
		zap.String("tx_hash", tx.TxHash().String()))

	return nil
}

func (si *StakingIndexer) processWithdrawTx(tx *wire.MsgTx, stakingTxHash *chainhash.Hash, unbondingTxHash *chainhash.Hash) error {
	if unbondingTxHash == nil {
		si.logger.Info("found a withdraw tx from staking",
			zap.String("tx_hash", tx.TxHash().String()),
			zap.String("staking_tx_hash", stakingTxHash.String()),
		)
	} else {
		si.logger.Info("found a withdraw tx from unbonding",
			zap.String("tx_hash", tx.TxHash().String()),
			zap.String("staking_tx_hash", stakingTxHash.String()),
			zap.String("unbonding_tx_hash", unbondingTxHash.String()),
		)
	}

	withdrawEvent := queuecli.NewWithdrawStakingEvent(stakingTxHash.String())

	if err := si.consumer.PushWithdrawEvent(&withdrawEvent); err != nil {
		return fmt.Errorf("failed to push the withdraw event to the consumer: %w", err)
	}

	si.logger.Info("successfully pushing the withdraw event",
		zap.String("tx_hash", tx.TxHash().String()))

	return nil
}

func (si *StakingIndexer) tryParseStakingTx(tx *wire.MsgTx) (*btcstaking.ParsedV0StakingTx, error) {
	possible := btcstaking.IsPossibleV0StakingTx(tx, si.params.Tag)
	if !possible {
		return nil, fmt.Errorf("not staking tx")
	}

	parsedData, err := btcstaking.ParseV0StakingTx(
		tx,
		si.params.Tag,
		si.params.CovenantPks,
		si.params.CovenantQuorum,
		&si.cfg.BTCNetParams)
	if err != nil {
		return nil, fmt.Errorf("not staking tx")
	}

	return parsedData, nil
}

func (si *StakingIndexer) GetStakingTxByHash(hash *chainhash.Hash) (*indexerstore.StoredStakingTransaction, error) {
	return si.is.GetStakingTransaction(hash)
}

func (si *StakingIndexer) GetUnbondingTxByHash(hash *chainhash.Hash) (*indexerstore.StoredUnbondingTransaction, error) {
	return si.is.GetUnbondingTransaction(hash)
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

func getTxHex(tx *wire.MsgTx) (string, error) {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", fmt.Errorf("failed to serialize the tx: %w", err)
	}
	txHex := hex.EncodeToString(buf.Bytes())

	return txHex, nil
}

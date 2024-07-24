package indexer

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/babylonchain/babylon/btcstaking"
	"github.com/babylonchain/networks/parameters/parser"

	// queuecli "github.com/babylonchain/staking-queue-client/client"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/kvdb"
	queuecli "github.com/scalarorg/staking-queue-client/client"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/btcscanner"
	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/consumer"

	// "github.com/babylonchain/staking-indexer/indexerstore"
	"github.com/babylonchain/staking-indexer/types"
	"github.com/scalarorg/staking-indexer/indexerstore"
)

type StakingIndexer struct {
	startOnce sync.Once
	stopOnce  sync.Once

	consumer       consumer.EventConsumer
	paramsVersions *parser.ParsedGlobalParams

	cfg    *config.Config
	logger *zap.Logger

	is *indexerstore.IndexerStore

	btcScanner btcscanner.BtcScanner

	wg   sync.WaitGroup
	quit chan struct{}
}

func NewStakingIndexer(
	cfg *config.Config,
	logger *zap.Logger,
	consumer consumer.EventConsumer,
	db kvdb.Backend,
	paramsVersions *parser.ParsedGlobalParams,
	btcScanner btcscanner.BtcScanner,
) (*StakingIndexer, error) {
	is, err := indexerstore.NewIndexerStore(db)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate staking indexer store: %w", err)
	}

	return &StakingIndexer{
		cfg:            cfg,
		logger:         logger.With(zap.String("module", "staking indexer")),
		consumer:       consumer,
		is:             is,
		paramsVersions: paramsVersions,
		btcScanner:     btcScanner,
		quit:           make(chan struct{}),
	}, nil
}

// Start starts the staking indexer core
func (si *StakingIndexer) Start(startHeight uint64) error {
	var startErr error
	si.startOnce.Do(func() {
		si.logger.Info("Starting Staking Indexer App")

		si.wg.Add(1)
		// go si.blocksEventLoop()
		// TODO_SCALAR: Done
		go si.blocksScalarEventLoop()

		if err := si.ValidateStartHeight(startHeight); err != nil {
			startErr = fmt.Errorf("invalid start height %d: %w", startHeight, err)
			return
		}

		if err := si.btcScanner.Start(startHeight, si.paramsVersions.Versions[0].ActivationHeight); err != nil {
			startErr = err
			return
		}

		// record metrics
		startBtcHeight.Set(float64(startHeight))

		si.logger.Info("Staking Indexer App is successfully started!")
	})

	return startErr
}

// ValidateStartHeight validates the given startHeight and returns an error
// if the given startHeight is not in the range of
// [earliest activation height, last processed height + 1]
// The point of this validation is to ensure the indexer
// (1) does not handle irrelevant blocks (impossible to have staking tx)
// (2) does not miss relevant blocks (possible to have staking tx)
func (si *StakingIndexer) ValidateStartHeight(startHeight uint64) error {
	baseHeight := si.paramsVersions.Versions[0].ActivationHeight
	if startHeight < baseHeight {
		return fmt.Errorf("the start height should not be lower than the earliest activation height %d", baseHeight)
	}

	lastProcessedHeight, err := si.is.GetLastProcessedHeight()
	if err != nil && startHeight != baseHeight {
		return fmt.Errorf("the database is empty, the start height should be equal to the base height %d", baseHeight)
	}

	if lastProcessedHeight != 0 && startHeight > lastProcessedHeight+1 {
		return fmt.Errorf("the start height should not be higher than %d (the last processed height + 1)", lastProcessedHeight+1)
	}

	return nil
}

// GetStartHeight returns a start height that can pass ValidateStartHeight()
// if the database is empty, then the base height in the config will be returned
// otherwise, it will return the last processed height + 1
func (si *StakingIndexer) GetStartHeight() uint64 {
	lastProcessedHeight, err := si.is.GetLastProcessedHeight()
	if err != nil {
		return si.paramsVersions.Versions[0].ActivationHeight
	}

	return lastProcessedHeight + 1
}

func (si *StakingIndexer) blocksEventLoop() {
	defer si.wg.Done()

	for {
		select {
		case update := <-si.btcScanner.ChainUpdateInfoChan():
			confirmedBlocks := update.ConfirmedBlocks
			for _, block := range confirmedBlocks {
				si.logger.Info("received confirmed block",
					zap.Int32("height", block.Height))

				if err := si.HandleConfirmedBlock(block); err != nil {
					// this indicates systematic failure
					si.logger.Fatal("failed to handle block",
						zap.Int32("height", block.Height),
						zap.Error(err))
				}
			}

			if err := si.processUnconfirmedInfo(update.UnconfirmedBlocks); err != nil {
				si.logger.Error("failed to process unconfirmed blocks",
					zap.Error(err))

				failedProcessingUnconfirmedBlockCounter.Inc()
			}

		case <-si.quit:
			si.logger.Info("closing the confirmed blocks loop")
			return
		}
	}
}

// processUnconfirmedInfo processes information from given unconfirmed blocks
// It follows the steps below:
// 1. iterate all txs of each unconfirmed block to identify staking and unbonding transactions,
// and calculate total unconfirmed tvl
// 2. get the current confirmed tvl
// 3. push unconfirmed info event to the queue
// 4. record metrics
// This method will not make any change to the system state.
func (si *StakingIndexer) processUnconfirmedInfo(unconfirmedBlocks []*types.IndexedBlock) error {
	if len(unconfirmedBlocks) == 0 {
		si.logger.Info("no unconfirmed blocks, skip processing unconfirmed info")
		return nil
	}

	si.logger.Info("processing unconfirmed blocks",
		zap.Int32("start_height", unconfirmedBlocks[0].Height),
		zap.Int32("end_height", unconfirmedBlocks[len(unconfirmedBlocks)-1].Height))

	tipBlockCache := unconfirmedBlocks[len(unconfirmedBlocks)-1]

	tvlInUnconfirmedBlocks, err := si.CalculateTvlInUnconfirmedBlocks(unconfirmedBlocks)
	if err != nil {
		return fmt.Errorf("failed to calculate unconfirmed tvl: %w", err)
	}

	confirmedTvl, err := si.GetConfirmedTvl()
	if err != nil {
		return fmt.Errorf("failed to get the confirmed TVL: %w", err)
	}

	unconfirmedTvl := btcutil.Amount(confirmedTvl) + tvlInUnconfirmedBlocks
	if unconfirmedTvl < 0 {
		return fmt.Errorf("total tvl %d is negative", unconfirmedTvl)
	}

	si.logger.Info("successfully calculated unconfirmed TVL",
		zap.Int32("tip_height", tipBlockCache.Height),
		zap.Uint64("confirmed_tvl", confirmedTvl),
		zap.Int64("tvl_in_unconfirmed_blocks", int64(tvlInUnconfirmedBlocks)),
		zap.Int64("unconfirmed_tvl", int64(unconfirmedTvl)))

	btcInfoEvent := queuecli.NewBtcInfoEvent(uint64(tipBlockCache.Height), confirmedTvl, uint64(unconfirmedTvl))
	if err := si.consumer.PushBtcInfoEvent(&btcInfoEvent); err != nil {
		return fmt.Errorf("failed to push the unconfirmed event: %w", err)
	}

	// record metrics
	lastCalculatedTvl.Set(float64(unconfirmedTvl))

	return nil
}

func (si *StakingIndexer) CalculateTvlInUnconfirmedBlocks(unconfirmedBlocks []*types.IndexedBlock) (btcutil.Amount, error) {
	tvl := btcutil.Amount(0)
	unconfirmedStakingTxs := make(map[chainhash.Hash]*indexerstore.StoredStakingTransaction)
	for _, b := range unconfirmedBlocks {
		params, err := si.getVersionedParams(uint64(b.Height))
		if err != nil {
			return 0, err
		}

		for _, tx := range b.Txs {
			msgTx := tx.MsgTx()

			// 1. try to parse staking tx
			stakingData, err := si.tryParseStakingTx(msgTx, params)
			if err == nil {
				// this is a new staking tx, validate it against staking requirement
				if err := si.validateStakingTx(params, stakingData); err != nil {
					// Note: the metrics and logs will be repeated when the tx is confirmed
					invalidTransactionsCounter.WithLabelValues("unconfirmed_staking_transaction").Inc()
					si.logger.Warn("found an invalid staking tx",
						zap.String("tx_hash", msgTx.TxHash().String()),
						zap.Int32("height", b.Height),
						zap.Bool("is_confirmed", false),
						zap.Error(err),
					)

					// invalid staking tx will not be counted for TVL
					continue
				}

				tvl += btcutil.Amount(stakingData.StakingOutput.Value)
				// save the staking tx in memory for later identifying unbonding tx
				stakingValue := uint64(stakingData.StakingOutput.Value)
				unconfirmedStakingTxs[msgTx.TxHash()] = &indexerstore.StoredStakingTransaction{
					Tx:                 msgTx,
					StakingOutputIdx:   uint32(stakingData.StakingOutputIdx),
					InclusionHeight:    uint64(b.Height),
					StakerPk:           stakingData.OpReturnData.StakerPublicKey.PubKey,
					StakingTime:        uint32(stakingData.OpReturnData.StakingTime),
					FinalityProviderPk: stakingData.OpReturnData.FinalityProviderPublicKey.PubKey,
					StakingValue:       stakingValue,
				}

				si.logger.Info("found an unconfirmed staking tx",
					zap.String("tx_hash", msgTx.TxHash().String()),
					zap.Uint64("value", stakingValue),
					zap.Int32("height", b.Height))

				continue
			}

			// 2. not a staking tx, check whether it spends a stored staking tx
			stakingTxs, _ := si.getSpentStakingTxs(msgTx)
			if len(stakingTxs) == 0 {
				// it does not spend a stored staking tx, check whether it spends
				// an unconfirmed staking tx
				stakingTxs, _ = getSpentFromStakingTxs(msgTx, unconfirmedStakingTxs)
			}
			for _, stakingTx := range stakingTxs {
				// 3. is a spending tx, check whether it is a valid unbonding tx
				paramsFromStakingTxHeight, err := si.getVersionedParams(stakingTx.InclusionHeight)
				if err != nil {
					return 0, err
				}
				isUnbonding, err := si.IsValidUnbondingTx(msgTx, stakingTx, paramsFromStakingTxHeight)
				if err != nil {
					if errors.Is(err, ErrInvalidUnbondingTx) {
						invalidTransactionsCounter.WithLabelValues("unconfirmed_unbonding_transactions").Inc()
						si.logger.Warn("found an invalid unbonding tx",
							zap.String("tx_hash", msgTx.TxHash().String()),
							zap.Int32("height", b.Height),
							zap.Bool("is_confirmed", false),
							zap.Error(err),
						)

						continue
					}

					// record metrics
					failedVerifyingUnbondingTxsCounter.Inc()
					return 0, fmt.Errorf("failed to validate unconfirmed unbonding tx: %w", err)
				}
				if isUnbonding {
					si.logger.Info("found an unconfirmed unbonding tx",
						zap.String("tx_hash", msgTx.TxHash().String()),
						zap.String("staking_tx_hash", stakingTx.Tx.TxHash().String()),
						zap.Uint64("value", stakingTx.StakingValue))

					// only subtract the tvl if the staking tx is not overflow
					if !stakingTx.IsOverflow {
						tvl -= btcutil.Amount(stakingTx.StakingValue)
					}
				} else {
					// TODO 1. Identify withdraw txs
					// TODO 2. Decide whether to subtract tvl here
					invalidTransactionsCounter.WithLabelValues("unconfirmed_unknown_transaction").Inc()

					si.logger.Warn("found a tx that spends the staking tx but not an unbonding tx",
						zap.String("tx_hash", msgTx.TxHash().String()),
						zap.String("staking_tx_hash", stakingTx.Tx.TxHash().String()))
				}
			}
		}
	}

	return tvl, nil
}

// HandleConfirmedBlock iterates through the tx set of a confirmed block and
// parse the staking, unbonding, and withdrawal txs if there are any.
func (si *StakingIndexer) HandleConfirmedBlock(b *types.IndexedBlock) error {
	params, err := si.getVersionedParams(uint64(b.Height))
	if err != nil {
		return err
	}
	for _, tx := range b.Txs {
		msgTx := tx.MsgTx()
		// 1. try to parse staking tx
		stakingData, err := si.tryParseStakingTx(msgTx, params)
		if err == nil {
			if err := si.ProcessStakingTx(
				msgTx, stakingData, uint64(b.Height), b.Header.Timestamp, params,
			); err != nil {
				// record metrics
				failedProcessingStakingTxsCounter.Inc()
				return fmt.Errorf("failed to process the staking tx: %w", err)
			}

			// should not use *continue* here as a special case is
			// the tx could be a staking tx as well as a withdrawal
			// tx that spends the previous staking tx
		}

		// 2. not a staking tx, check whether it is a spending tx from a previous
		// staking tx, and handle it if so
		stakingTxs, spendStakingInputIndexes := si.getSpentStakingTxs(msgTx)
		for i, stakingTx := range stakingTxs {
			// this is a spending tx from a previous staking tx, further process it
			// by checking whether it is unbonding or withdrawal
			if err := si.handleSpendingStakingTransaction(
				msgTx, stakingTx, spendStakingInputIndexes[i],
				uint64(b.Height), b.Header.Timestamp); err != nil {

				return err
			}
		}

		// 3. it's not a spending tx from a previous staking tx,
		// check whether it spends a previous unbonding tx, and
		// handle it if so
		unbondingTxs, spendUnbondingInputIndexes := si.getSpentUnbondingTxs(msgTx)
		for i, unbondingTx := range unbondingTxs {
			// this is a spending tx from the unbonding, validate it, and processes it
			if err := si.handleSpendingUnbondingTransaction(
				msgTx, unbondingTx, spendUnbondingInputIndexes[i], uint64(b.Height)); err != nil {

				return err
			}
		}
	}

	if err := si.is.SaveLastProcessedHeight(uint64(b.Height)); err != nil {
		return fmt.Errorf("failed to save the last processed height: %w", err)
	}

	// record metrics
	lastProcessedBtcHeight.Set(float64(b.Height))
	return nil
}

func (si *StakingIndexer) handleSpendingUnbondingTransaction(
	tx *wire.MsgTx,
	unbondingTx *indexerstore.StoredUnbondingTransaction,
	spendingInputIdx int,
	height uint64,
) error {
	// get the stored staking tx for later validation
	storedStakingTx, err := si.GetStakingTxByHash(unbondingTx.StakingTxHash)
	if err != nil {
		// record metrics
		failedProcessingWithdrawTxsFromUnbondingCounter.Inc()

		return err
	}

	paramsFromStakingTxHeight, err := si.getVersionedParams(storedStakingTx.InclusionHeight)
	if err != nil {
		return err
	}

	if err := si.ValidateWithdrawalTxFromUnbonding(tx, storedStakingTx, spendingInputIdx, paramsFromStakingTxHeight); err != nil {
		if errors.Is(err, ErrInvalidWithdrawalTx) {
			// TODO consider slashing transaction for phase-2
			invalidTransactionsCounter.WithLabelValues("confirmed_withdraw_unbonding_transactions").Inc()
			si.logger.Warn("found an invalid withdrawal tx from unbonding",
				zap.String("tx_hash", tx.TxHash().String()),
				zap.Uint64("height", height),
				zap.Bool("is_confirmed", true),
				zap.Error(err),
			)

			return nil
		}

		failedProcessingWithdrawTxsFromUnbondingCounter.Inc()
		return err
	}

	unbondingTxHash := unbondingTx.Tx.TxHash()
	if err := si.processWithdrawTx(tx, unbondingTx.StakingTxHash, &unbondingTxHash, height); err != nil {
		// record metrics
		failedProcessingWithdrawTxsFromUnbondingCounter.Inc()

		return err
	}

	return nil
}

func (si *StakingIndexer) handleSpendingStakingTransaction(
	tx *wire.MsgTx,
	stakingTx *indexerstore.StoredStakingTransaction,
	spendingInputIndex int,
	height uint64,
	timestamp time.Time,
) error {
	stakingTxHash := stakingTx.Tx.TxHash()
	paramsFromStakingTxHeight, err := si.getVersionedParams(stakingTx.InclusionHeight)
	if err != nil {
		return err
	}

	// check whether it is a valid unbonding tx
	isUnbonding, err := si.IsValidUnbondingTx(tx, stakingTx, paramsFromStakingTxHeight)
	if err != nil {
		if errors.Is(err, ErrInvalidUnbondingTx) {
			invalidTransactionsCounter.WithLabelValues("confirmed_unbonding_transactions").Inc()
			si.logger.Warn("found an invalid unbonding tx",
				zap.String("tx_hash", tx.TxHash().String()),
				zap.Uint64("height", height),
				zap.Bool("is_confirmed", true),
				zap.Error(err),
			)

			return nil
		}
		// record metrics
		failedVerifyingUnbondingTxsCounter.Inc()
		return err
	}

	if !isUnbonding {
		// not an unbonding tx, so this is a withdraw tx from the staking,
		// validate it and process it
		if err := si.ValidateWithdrawalTxFromStaking(tx, stakingTx, spendingInputIndex, paramsFromStakingTxHeight); err != nil {
			if errors.Is(err, ErrInvalidWithdrawalTx) {
				invalidTransactionsCounter.WithLabelValues("confirmed_withdraw_staking_transactions").Inc()
				si.logger.Warn("found an invalid withdrawal tx from staking",
					zap.String("tx_hash", tx.TxHash().String()),
					zap.Uint64("height", height),
					zap.Bool("is_confirmed", true),
					zap.Error(err),
				)

				return nil
			}

			failedProcessingWithdrawTxsFromStakingCounter.Inc()
			return err
		}
		if err := si.processWithdrawTx(tx, &stakingTxHash, nil, height); err != nil {
			// record metrics
			failedProcessingWithdrawTxsFromStakingCounter.Inc()

			return err
		}
		return nil
	}

	// 5. this is a valid unbonding tx, process it
	if err := si.ProcessUnbondingTx(
		tx, &stakingTxHash, height, timestamp,
		paramsFromStakingTxHeight,
	); err != nil {
		if !errors.Is(err, indexerstore.ErrDuplicateTransaction) {
			// record metrics
			failedProcessingUnbondingTxsCounter.Inc()

			return err
		}
		// we don't consider duplicate error critical as it can happen
		// when the indexer restarts
		si.logger.Warn("found a duplicate tx",
			zap.String("tx_hash", tx.TxHash().String()))
	}

	return nil
}

func (si *StakingIndexer) ValidateWithdrawalTxFromStaking(
	tx *wire.MsgTx,
	stakingTx *indexerstore.StoredStakingTransaction,
	spendingInputIdx int,
	params *parser.ParsedVersionedGlobalParams,
) error {
	// re-build the time-lock path script and check whether the script from
	// the witness matches
	stakingInfo, err := btcstaking.BuildStakingInfo(
		stakingTx.StakerPk,
		[]*btcec.PublicKey{stakingTx.FinalityProviderPk},
		params.CovenantPks,
		params.CovenantQuorum,
		uint16(stakingTx.StakingTime),
		btcutil.Amount(stakingTx.StakingValue),
		&si.cfg.BTCNetParams,
	)
	if err != nil {
		return fmt.Errorf("failed to rebuid the staking info: %w", err)
	}
	timelockPathInfo, err := stakingInfo.TimeLockPathSpendInfo()
	if err != nil {
		return fmt.Errorf("failed to get the unbonding path spend info: %w", err)
	}

	witness := tx.TxIn[spendingInputIdx].Witness
	if len(witness) < 2 {
		panic(fmt.Errorf("spending tx should have at least 2 elements in witness, got %d", len(witness)))
	}

	scriptFromWitness := tx.TxIn[spendingInputIdx].Witness[len(tx.TxIn[spendingInputIdx].Witness)-2]

	if !bytes.Equal(timelockPathInfo.GetPkScriptPath(), scriptFromWitness) {
		return fmt.Errorf("%w: the tx does not unlock the time-lock path", ErrInvalidWithdrawalTx)
	}

	return nil
}

func (si *StakingIndexer) ValidateWithdrawalTxFromUnbonding(
	tx *wire.MsgTx,
	stakingTx *indexerstore.StoredStakingTransaction,
	spendingInputIdx int,
	params *parser.ParsedVersionedGlobalParams,
) error {
	// re-build the time-lock path script and check whether the script from
	// the witness matches
	expectedUnbondingOutputValue := btcutil.Amount(stakingTx.StakingValue) - params.UnbondingFee
	unbondingInfo, err := btcstaking.BuildUnbondingInfo(
		stakingTx.StakerPk,
		[]*btcec.PublicKey{stakingTx.FinalityProviderPk},
		params.CovenantPks,
		params.CovenantQuorum,
		params.UnbondingTime,
		expectedUnbondingOutputValue,
		&si.cfg.BTCNetParams,
	)
	if err != nil {
		return fmt.Errorf("failed to rebuid the unbonding info: %w", err)
	}
	timelockPathInfo, err := unbondingInfo.TimeLockPathSpendInfo()
	if err != nil {
		return fmt.Errorf("failed to get the unbonding path spend info: %w", err)
	}

	witness := tx.TxIn[spendingInputIdx].Witness
	if len(witness) < 2 {
		panic(fmt.Errorf("spending tx should have at least 2 elements in witness, got %d", len(witness)))
	}

	scriptFromWitness := tx.TxIn[spendingInputIdx].Witness[len(tx.TxIn[spendingInputIdx].Witness)-2]

	if !bytes.Equal(timelockPathInfo.GetPkScriptPath(), scriptFromWitness) {
		return fmt.Errorf("%w: the tx does not unlock the time-lock path", ErrInvalidWithdrawalTx)
	}

	return nil
}

func (si *StakingIndexer) IsTxProcessed(txHash *chainhash.Hash) (bool, error) {
	return si.is.TxExists(txHash)
}

// getSpentStakingTxs find all the stored staking txs spent by the given tx.
// It returns the found staking txs and the spending input index of the given tx
func (si *StakingIndexer) getSpentStakingTxs(tx *wire.MsgTx) ([]*indexerstore.StoredStakingTransaction, []int) {
	storedStakingTxs := make([]*indexerstore.StoredStakingTransaction, 0)
	spendingInputIndexes := make([]int, 0)
	for i, txIn := range tx.TxIn {
		maybeStakingTxHash := txIn.PreviousOutPoint.Hash
		stakingTx, err := si.GetStakingTxByHash(&maybeStakingTxHash)
		if err != nil || stakingTx == nil {
			continue
		}

		// this ensures the spending tx spends the correct staking output
		if txIn.PreviousOutPoint.Index != stakingTx.StakingOutputIdx {
			continue
		}

		storedStakingTxs = append(storedStakingTxs, stakingTx)
		spendingInputIndexes = append(spendingInputIndexes, i)
	}

	return storedStakingTxs, spendingInputIndexes
}

// getSpentStakingTxs find all the staking txs from the given ones spent by the given tx.
// It returns the found staking txs and the spending input index of the given tx
func getSpentFromStakingTxs(
	tx *wire.MsgTx,
	stakingTxs map[chainhash.Hash]*indexerstore.StoredStakingTransaction,
) ([]*indexerstore.StoredStakingTransaction, []int) {
	storedStakingTxs := make([]*indexerstore.StoredStakingTransaction, 0)
	spendingInputIndexes := make([]int, 0)
	for i, txIn := range tx.TxIn {
		maybeStakingTxHash := txIn.PreviousOutPoint.Hash
		stakingTx, exists := stakingTxs[maybeStakingTxHash]
		if !exists {
			continue
		}

		// this ensures the spending tx spends the correct staking output
		if txIn.PreviousOutPoint.Index != stakingTx.StakingOutputIdx {
			continue
		}

		storedStakingTxs = append(storedStakingTxs, stakingTx)
		spendingInputIndexes = append(spendingInputIndexes, i)
	}

	return storedStakingTxs, spendingInputIndexes
}

// getSpentUnbondingTxs find all the stored unbonding txs spent by the given tx.
// It returns the found unbonding txs and the spending input index of the given tx
func (si *StakingIndexer) getSpentUnbondingTxs(tx *wire.MsgTx) ([]*indexerstore.StoredUnbondingTransaction, []int) {
	storedUnbondingTxs := make([]*indexerstore.StoredUnbondingTransaction, 0)
	spendingInputIndexes := make([]int, 0)
	for i, txIn := range tx.TxIn {
		maybeUnbondingTxHash := txIn.PreviousOutPoint.Hash
		unbondingTx, err := si.GetUnbondingTxByHash(&maybeUnbondingTxHash)
		if err != nil || unbondingTx == nil {
			continue
		}

		storedUnbondingTxs = append(storedUnbondingTxs, unbondingTx)
		spendingInputIndexes = append(spendingInputIndexes, i)
	}

	return storedUnbondingTxs, spendingInputIndexes
}

// IsValidUnbondingTx tries to identify a tx is a valid unbonding tx
// It returns error when (1) it fails to verify the unbonding tx due
// to invalid parameters, and (2) the tx spends the unbonding path
// but is invalid
func (si *StakingIndexer) IsValidUnbondingTx(tx *wire.MsgTx, stakingTx *indexerstore.StoredStakingTransaction, params *parser.ParsedVersionedGlobalParams) (bool, error) {
	// 1. an unbonding tx must be a transfer tx
	if err := btcstaking.IsTransferTx(tx); err != nil {
		return false, nil
	}

	// 2. an unbonding tx must spend the staking output
	stakingTxHash := stakingTx.Tx.TxHash()
	if !tx.TxIn[0].PreviousOutPoint.Hash.IsEqual(&stakingTxHash) {
		return false, nil
	}
	if tx.TxIn[0].PreviousOutPoint.Index != stakingTx.StakingOutputIdx {
		return false, nil
	}

	// 3. re-build the unbonding path script and check whether the script from
	// the witness matches
	stakingInfo, err := btcstaking.BuildStakingInfo(
		stakingTx.StakerPk,
		[]*btcec.PublicKey{stakingTx.FinalityProviderPk},
		params.CovenantPks,
		params.CovenantQuorum,
		uint16(stakingTx.StakingTime),
		btcutil.Amount(stakingTx.StakingValue),
		&si.cfg.BTCNetParams,
	)
	if err != nil {
		return false, fmt.Errorf("failed to rebuid the staking info: %w", err)
	}
	unbondingPathInfo, err := stakingInfo.UnbondingPathSpendInfo()
	if err != nil {
		return false, fmt.Errorf("failed to get the unbonding path spend info: %w", err)
	}

	witness := tx.TxIn[0].Witness
	if len(witness) < 2 {
		panic(fmt.Errorf("spending tx should have at least 2 elements in witness, got %d", len(witness)))
	}

	scriptFromWitness := tx.TxIn[0].Witness[len(tx.TxIn[0].Witness)-2]

	if !bytes.Equal(unbondingPathInfo.GetPkScriptPath(), scriptFromWitness) {
		// not unbonding tx as it does not unlock the unbonding path
		return false, nil
	}

	// 4. check whether the unbonding tx enables rbf has time lock
	if tx.TxIn[0].Sequence != wire.MaxTxInSequenceNum {
		return false, fmt.Errorf("%w: unbonding tx should not enable rbf", ErrInvalidUnbondingTx)
	}
	if tx.LockTime != 0 {
		return false, fmt.Errorf("%w: unbonding tx should not set lock time", ErrInvalidUnbondingTx)
	}

	// 5. check whether the script of an unbonding tx output is expected
	// by re-building unbonding output from params
	stakingValue := btcutil.Amount(stakingTx.Tx.TxOut[stakingTx.StakingOutputIdx].Value)
	expectedUnbondingOutputValue := stakingValue - params.UnbondingFee
	if expectedUnbondingOutputValue <= 0 {
		return false, fmt.Errorf("%w: staking output value is too low, got %v, unbonding fee: %v",
			ErrInvalidUnbondingTx, stakingValue, params.UnbondingFee)
	}
	unbondingInfo, err := btcstaking.BuildUnbondingInfo(
		stakingTx.StakerPk,
		[]*btcec.PublicKey{stakingTx.FinalityProviderPk},
		params.CovenantPks,
		params.CovenantQuorum,
		params.UnbondingTime,
		expectedUnbondingOutputValue,
		&si.cfg.BTCNetParams,
	)
	if err != nil {
		return false, fmt.Errorf("failed to rebuid the unbonding info: %w", err)
	}
	if !bytes.Equal(tx.TxOut[0].PkScript, unbondingInfo.UnbondingOutput.PkScript) {
		return false, fmt.Errorf("%w: the unbonding output is not expected", ErrInvalidUnbondingTx)
	}
	if tx.TxOut[0].Value != unbondingInfo.UnbondingOutput.Value {
		return false, fmt.Errorf("%w: the unbonding output value %d is not expected %d",
			ErrInvalidUnbondingTx, tx.TxOut[0].Value, unbondingInfo.UnbondingOutput.Value)
	}

	return true, nil
}

func (si *StakingIndexer) ProcessStakingTx(
	tx *wire.MsgTx,
	stakingData *btcstaking.ParsedV0StakingTx,
	height uint64, timestamp time.Time,
	params *parser.ParsedVersionedGlobalParams,
) error {
	var (
		// whether the staking tx is overflow
		isOverflow bool
	)
	si.logger.Info("found a staking tx",
		zap.Uint64("height", height),
		zap.String("tx_hash", tx.TxHash().String()),
		zap.Int64("value", stakingData.StakingOutput.Value),
	)
	// check whether the staking tx already exists in db
	// if so, get the isOverflow from the data in db
	// otherwise, check it if the current tvl already reaches
	// the cap
	txHash := tx.TxHash()
	storedStakingTx, err := si.is.GetStakingTransaction(&txHash)
	if err != nil {
		return err
	}
	if storedStakingTx != nil {
		isOverflow = storedStakingTx.IsOverflow
	} else {
		// this is a new staking tx, validate it against staking requirement
		if err := si.validateStakingTx(params, stakingData); err != nil {
			invalidTransactionsCounter.WithLabelValues("confirmed_staking_transaction").Inc()
			si.logger.Warn("found an invalid staking tx",
				zap.String("tx_hash", tx.TxHash().String()),
				zap.Uint64("height", height),
				zap.Bool("is_confirmed", true),
				zap.Error(err),
			)
			// TODO handle invalid staking tx (storing and pushing events)
			return nil
		}

		// check if the staking tvl is overflow with this staking tx
		stakingOverflow, err := si.isOverflow(height, params)
		if err != nil {
			return fmt.Errorf("failed to check the overflow of staking tx: %w", err)
		}

		isOverflow = stakingOverflow
	}

	if isOverflow {
		si.logger.Info("the staking tx is overflow",
			zap.String("tx_hash", tx.TxHash().String()))
	}

	// add the staking transaction to the system state
	if err := si.addStakingTransaction(
		height, timestamp, tx,
		stakingData.OpReturnData.StakerPublicKey.PubKey,
		stakingData.OpReturnData.FinalityProviderPublicKey.PubKey,
		uint64(stakingData.StakingOutput.Value),
		uint32(stakingData.OpReturnData.StakingTime),
		uint32(stakingData.StakingOutputIdx),
		isOverflow,
	); err != nil {
		return err
	}

	return nil
}

// addStakingTransaction pushes the staking event, saves it to the database
// and records metrics
func (si *StakingIndexer) addStakingTransaction(
	height uint64,
	timestamp time.Time,
	tx *wire.MsgTx,
	stakerPk *btcec.PublicKey,
	fpPk *btcec.PublicKey,
	stakingValue uint64,
	stakingTime uint32,
	stakingOutputIndex uint32,
	isOverflow bool,
) error {
	txHex, err := getTxHex(tx)
	if err != nil {
		return err
	}

	stakingEvent := queuecli.NewActiveStakingEvent(
		tx.TxHash().String(),
		hex.EncodeToString(schnorr.SerializePubKey(stakerPk)),
		hex.EncodeToString(schnorr.SerializePubKey(fpPk)),
		stakingValue,
		height,
		timestamp.Unix(),
		uint64(stakingTime),
		uint64(stakingOutputIndex),
		txHex,
		isOverflow,
	)

	// push the events first then save the tx due to the assumption
	// that the consumer can handle duplicate events
	if err := si.consumer.PushStakingEvent(&stakingEvent); err != nil {
		return fmt.Errorf("failed to push the staking event to the queue: %w", err)
	}

	si.logger.Info("saving the staking transaction",
		zap.String("tx_hash", tx.TxHash().String()),
	)

	// save the staking tx in the db
	if err := si.is.AddStakingTransaction(
		tx, stakingOutputIndex, height,
		stakerPk, stakingTime, fpPk,
		stakingValue, isOverflow,
	); err != nil && !errors.Is(err, indexerstore.ErrDuplicateTransaction) {
		return fmt.Errorf("failed to add the staking tx to store: %w", err)
	}

	si.logger.Info("successfully saved the staking transaction",
		zap.String("tx_hash", tx.TxHash().String()),
	)

	// record metrics
	if isOverflow {
		totalStakingTxs.WithLabelValues("overflow").Inc()
	} else {
		totalStakingTxs.WithLabelValues("active").Inc()
	}
	lastFoundStakingTxHeight.Set(float64(height))

	return nil
}

func (si *StakingIndexer) ProcessUnbondingTx(
	tx *wire.MsgTx,
	stakingTxHash *chainhash.Hash,
	height uint64, timestamp time.Time,
	params *parser.ParsedVersionedGlobalParams,
) error {
	si.logger.Info("found an unbonding tx",
		zap.Uint64("height", height),
		zap.String("tx_hash", tx.TxHash().String()),
		zap.String("staking_tx_hash", stakingTxHash.String()),
	)

	unbondingTxHex, err := getTxHex(tx)
	if err != nil {
		return err
	}

	unbondingTxHash := tx.TxHash()
	unbondingEvent := queuecli.NewUnbondingStakingEvent(
		stakingTxHash.String(),
		height,
		timestamp.Unix(),
		uint64(params.UnbondingTime),
		// valid unbonding tx always has one output
		0,
		unbondingTxHex,
		unbondingTxHash.String(),
	)

	if err := si.consumer.PushUnbondingEvent(&unbondingEvent); err != nil {
		return fmt.Errorf("failed to push the unbonding event to the queue: %w", err)
	}

	si.logger.Info("saving the unbonding tx",
		zap.String("tx_hash", unbondingTxHash.String()))

	if err := si.is.AddUnbondingTransaction(
		tx,
		stakingTxHash,
	); err != nil && !errors.Is(err, indexerstore.ErrDuplicateTransaction) {
		return fmt.Errorf("failed to add the unbonding tx to store: %w", err)
	}

	si.logger.Info("successfully saved the unbonding tx",
		zap.String("tx_hash", tx.TxHash().String()))

	// record metrics
	totalUnbondingTxs.Inc()
	lastFoundUnbondingTxHeight.Set(float64(height))

	return nil
}

func (si *StakingIndexer) processWithdrawTx(tx *wire.MsgTx, stakingTxHash *chainhash.Hash, unbondingTxHash *chainhash.Hash, height uint64) error {
	txHashHex := tx.TxHash().String()
	if unbondingTxHash == nil {
		si.logger.Info("found a withdraw tx from staking",
			zap.String("tx_hash", txHashHex),
			zap.String("staking_tx_hash", stakingTxHash.String()),
		)
	} else {
		si.logger.Info("found a withdraw tx from unbonding",
			zap.String("tx_hash", txHashHex),
			zap.String("staking_tx_hash", stakingTxHash.String()),
			zap.String("unbonding_tx_hash", unbondingTxHash.String()),
		)
	}

	withdrawEvent := queuecli.NewWithdrawStakingEvent(stakingTxHash.String())

	if err := si.consumer.PushWithdrawEvent(&withdrawEvent); err != nil {
		return fmt.Errorf("failed to push the withdraw event to the consumer: %w", err)
	}

	// record metrics
	if unbondingTxHash == nil {
		totalWithdrawTxsFromStaking.Inc()
		lastFoundWithdrawTxFromStakingHeight.Set(float64(height))
	} else {
		totalWithdrawTxsFromUnbonding.Inc()
		lastFoundWithdrawTxFromUnbondingHeight.Set(float64(height))
	}

	return nil
}

func (si *StakingIndexer) tryParseStakingTx(tx *wire.MsgTx, params *parser.ParsedVersionedGlobalParams) (*btcstaking.ParsedV0StakingTx, error) {
	possible := btcstaking.IsPossibleV0StakingTx(tx, params.Tag)
	if !possible {
		return nil, fmt.Errorf("not staking tx")
	}

	parsedData, err := btcstaking.ParseV0StakingTx(
		tx,
		params.Tag,
		params.CovenantPks,
		params.CovenantQuorum,
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

		if err := si.btcScanner.Stop(); err != nil {
			stopErr = err
			return
		}

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

// validateStakingTx performs the validation checks for the staking tx
// such as min and max staking amount and staking time
func (si *StakingIndexer) validateStakingTx(params *parser.ParsedVersionedGlobalParams, stakingData *btcstaking.ParsedV0StakingTx) error {
	value := btcutil.Amount(stakingData.StakingOutput.Value)
	// Minimum staking amount check
	if value < params.MinStakingAmount {
		return fmt.Errorf("%w: staking amount is too low, expected: %v, got: %v",
			ErrInvalidStakingTx, params.MinStakingAmount, value)
	}

	// Maximum staking amount check
	if value > params.MaxStakingAmount {
		return fmt.Errorf("%w: staking amount is too high, expected: %v, got: %v",
			ErrInvalidStakingTx, params.MaxStakingAmount, value)
	}

	// Maximum staking time check
	if uint64(stakingData.OpReturnData.StakingTime) > uint64(params.MaxStakingTime) {
		return fmt.Errorf("%w: staking time is too high, expected: %v, got: %v",
			ErrInvalidStakingTx, params.MaxStakingTime, stakingData.OpReturnData.StakingTime)
	}

	// Minimum staking time check
	if uint64(stakingData.OpReturnData.StakingTime) < uint64(params.MinStakingTime) {
		return fmt.Errorf("%w: staking time is too low, expected: %v, got: %v",
			ErrInvalidStakingTx, params.MinStakingTime, stakingData.OpReturnData.StakingTime)
	}

	return nil
}

func (si *StakingIndexer) isOverflow(height uint64, params *parser.ParsedVersionedGlobalParams) (bool, error) {
	isTimeBased := params.CapHeight != 0

	if isTimeBased && height < params.ActivationHeight {
		panic(fmt.Errorf("the transaction height %d should not be lower than the param activation height: %d",
			height, params.ActivationHeight))
	}

	if isTimeBased && height > params.CapHeight {
		return true, nil
	}

	if isTimeBased && height <= params.CapHeight {
		return false, nil
	}

	confirmedTvl, err := si.is.GetConfirmedTvl()
	if err != nil {
		return false, fmt.Errorf("failed to get the confirmed TVL: %w", err)
	}

	return confirmedTvl >= uint64(params.StakingCap), nil
}

func (si *StakingIndexer) GetConfirmedTvl() (uint64, error) {
	return si.is.GetConfirmedTvl()
}

func (si *StakingIndexer) getVersionedParams(height uint64) (*parser.ParsedVersionedGlobalParams, error) {
	params := si.paramsVersions.GetVersionedGlobalParamsByHeight(height)
	if params == nil {
		return nil, fmt.Errorf("the params for height %d does not exist", height)
	}

	return params, nil
}

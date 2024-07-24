package indexer

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/babylonchain/babylon/btcstaking"
	"github.com/babylonchain/networks/parameters/parser"

	// "github.com/babylonchain/staking-indexer/indexerstore"

	"github.com/babylonchain/staking-indexer/types"
	// queuecli "github.com/babylonchain/staking-queue-client/client"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/scalarorg/btcvault"
	queuecli "github.com/scalarorg/staking-queue-client/client"

	// "github.com/scalarorg/staking-indexer/indexerstore"
	"github.com/scalarorg/staking-indexer/indexerstore"
	"go.uber.org/zap"
)

func (si *StakingIndexer) blocksScalarEventLoop() {
	defer si.wg.Done()

	for {
		select {
		case update := <-si.btcScanner.ChainUpdateInfoChan():
			confirmedBlocks := update.ConfirmedBlocks
			for _, block := range confirmedBlocks {
				si.logger.Info("received confirmed block",
					zap.Int32("height", block.Height))

				if err := si.HandleConfirmedBlockScalar(block); err != nil {
					// this indicates systematic failure
					si.logger.Fatal("failed to handle block",
						zap.Int32("height", block.Height),
						zap.Error(err))
				}
			}

			if err := si.processUnconfirmedInfoScalar(update.UnconfirmedBlocks); err != nil {
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

// based on indexer.go - compatible with the scalar vault
func (si *StakingIndexer) processUnconfirmedInfoScalar(unconfirmedBlocks []*types.IndexedBlock) error {
	if len(unconfirmedBlocks) == 0 {
		si.logger.Info("no unconfirmed blocks, skip processing unconfirmed info")
		return nil
	}

	si.logger.Info("processing unconfirmed blocks",
		zap.Int32("start_height", unconfirmedBlocks[0].Height),
		zap.Int32("end_height", unconfirmedBlocks[len(unconfirmedBlocks)-1].Height))

	tipBlockCache := unconfirmedBlocks[len(unconfirmedBlocks)-1]

	tvlInUnconfirmedBlocks, err := si.CalculateTvlInUnconfirmedBlocksScalar(unconfirmedBlocks)
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

func (si *StakingIndexer) CalculateTvlInUnconfirmedBlocksScalar(unconfirmedBlocks []*types.IndexedBlock) (btcutil.Amount, error) {
	tvl := btcutil.Amount(0)
	unconfirmedStakingTxs := make(map[chainhash.Hash]*indexerstore.StoredVaultTransaction)
	for _, b := range unconfirmedBlocks {
		params, err := si.getVersionedParams(uint64(b.Height))
		if err != nil {
			return 0, err
		}

		for _, tx := range b.Txs {
			msgTx := tx.MsgTx()

			// 1. try to parse vault tx
			vaultData, err := si.tryParseVaultTx(msgTx, params)
			if err == nil {
				// this is a new vault tx, validate it against vault requirement
				if err := si.validateVaultTx(params, vaultData); err != nil {
					// Note: the metrics and logs will be repeated when the tx is confirmed
					invalidTransactionsCounter.WithLabelValues("unconfirmed_vault_transaction").Inc()
					si.logger.Warn("found an invalid vault tx",
						zap.String("tx_hash", msgTx.TxHash().String()),
						zap.Int32("height", b.Height),
						zap.Bool("is_confirmed", false),
						zap.Error(err),
					)

					// invalid vault tx will not be counted for TVL
					continue
				}

				tvl += btcutil.Amount(vaultData.VaultOutput.Value)
				// save the staking tx in memory for later identifying spending tx
				vaultValue := uint64(vaultData.VaultOutput.Value)
				unconfirmedStakingTxs[msgTx.TxHash()] = &indexerstore.StoredVaultTransaction{
					Tx:                          msgTx,
					StakingOutputIdx:            uint32(vaultData.VaultOutputIdx),
					InclusionHeight:             uint64(b.Height),
					StakerPk:                    vaultData.OpReturnData.StakerPublicKey.PubKey,
					StakingValue:                vaultValue,
					DAppPk:                      vaultData.OpReturnData.FinalityProviderPublicKey.PubKey,
					ChainID:                     vaultData.PayloadOpReturnData.ChainID,
					ChainIdUserAddress:          vaultData.PayloadOpReturnData.ChainIdUserAddress,
					ChainIdSmartContractAddress: vaultData.PayloadOpReturnData.ChainIdSmartContractAddress,
					MintingAmount:               vaultData.PayloadOpReturnData.Amount,
				}

				si.logger.Info("found an unconfirmed staking tx",
					zap.String("tx_hash", msgTx.TxHash().String()),
					zap.Uint64("value", vaultValue),
					zap.Int32("height", b.Height))

				continue
			}

			// 2. not a staking tx, check whether it spends a stored staking tx
			vaultTxs, _ := si.getSpentVaultTxs(msgTx)
			if len(vaultTxs) == 0 {
				// it does not spend a stored staking tx, check whether it spends
				// an unconfirmed staking tx
				vaultTxs, _ = getSpentFromVaultTxs(msgTx, unconfirmedStakingTxs)
			}
			for _, vaultTx := range vaultTxs {
				// 3. is a spending tx, check whether it is a valid burning tx
				paramsFromVaultTxHeight, err := si.getVersionedParams(vaultTx.InclusionHeight)
				if err != nil {
					return 0, err
				}
				isBurning, err := si.IsValidBurningTx(msgTx, vaultTx, paramsFromVaultTxHeight)
				if err != nil {
					if errors.Is(err, ErrInvalidUnbondingTx) {
						invalidTransactionsCounter.WithLabelValues("unconfirmed_burning_transactions").Inc()
						si.logger.Warn("found an invalid burning tx",
							zap.String("tx_hash", msgTx.TxHash().String()),
							zap.Int32("height", b.Height),
							zap.Bool("is_confirmed", false),
							zap.Error(err),
						)

						continue
					}

					// record metrics
					failedVerifyingBurningTxsCounter.Inc()
					return 0, fmt.Errorf("failed to validate unconfirmed burning tx: %w", err)
				}
				if isBurning {
					si.logger.Info("found an unconfirmed burning tx",
						zap.String("tx_hash", msgTx.TxHash().String()),
						zap.String("staking_tx_hash", vaultTx.Tx.TxHash().String()),
						zap.Uint64("value", vaultTx.StakingValue))

					// only subtract the tvl if the staking tx is not overflow
					if !vaultTx.IsOverflow {
						tvl -= btcutil.Amount(vaultTx.StakingValue)
					}
				} else {
					// TODO 1. Identify withdraw txs
					// TODO 2. Decide whether to subtract tvl here
					invalidTransactionsCounter.WithLabelValues("unconfirmed_unknown_transaction").Inc()

					si.logger.Warn("found a tx that spends the staking tx but not an burning tx",
						zap.String("tx_hash", msgTx.TxHash().String()),
						zap.String("staking_tx_hash", vaultTx.Tx.TxHash().String()))
				}
			}
		}
	}

	return tvl, nil
}

func (si *StakingIndexer) HandleConfirmedBlockScalar(b *types.IndexedBlock) error {
	params, err := si.getVersionedParams(uint64(b.Height))
	if err != nil {
		return err
	}
	for _, tx := range b.Txs {
		msgTx := tx.MsgTx()
		// 1. try to parse staking tx
		vaultData, err := si.tryParseVaultTx(msgTx, params)
		if err == nil {
			if err := si.ProcessVaultTx(
				msgTx, vaultData, uint64(b.Height), b.Header.Timestamp, params,
			); err != nil {
				// record metrics
				failedProcessingVaultTxsCounter.Inc()
				return fmt.Errorf("failed to process the vault tx: %w", err)
			}
			// should not use *continue* here as a special case is
			// the tx could be a staking tx as well as a withdrawal
			// tx that spends the previous staking tx

		}

		// 2. not a vault tx, check whether it is a spending tx from a previous
		// vault tx, and handle it if so
		vaultTxs, spendVaultInputIndexes := si.getSpentVaultTxs(msgTx)
		for i, vaultTx := range vaultTxs {
			// this is a spending tx from a previous staking tx, further process it
			// by checking whether it is unbonding or withdrawal
			if err := si.handleSpendingVaultTransaction(
				msgTx, vaultTx, spendVaultInputIndexes[i],
				uint64(b.Height), b.Header.Timestamp); err != nil {
				return err
			}
		}

		// 3. it's not a spending tx from a previous staking tx,
		// check whether it spends a previous unbonding tx, and
		// handle it if so
		burningTxs, spendBurningInputIndexes := si.getSpentBurningTxs(msgTx)
		for i, burningTx := range burningTxs {
			// this is a spending tx from the unbonding, validate it, and processes it
			if err := si.handleSpendingBurningTransaction(
				msgTx, burningTx, spendBurningInputIndexes[i], uint64(b.Height)); err != nil {
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

func (si *StakingIndexer) handleSpendingBurningTransaction(
	tx *wire.MsgTx,
	burningTx *indexerstore.StoredBurningTransaction,
	spendingInputIdx int,
	height uint64,
) error {
	// get the stored staking tx for later validation
	storedVaultTx, err := si.GetVaultTxByHash(burningTx.VaultTxHash)
	if err != nil {
		// record metrics
		failedProcessingWithdrawTxsFromBurningCounter.Inc()

		return err
	}

	paramsFromVaultTxHeight, err := si.getVersionedParams(storedVaultTx.InclusionHeight)
	if err != nil {
		return err
	}

	if err := si.ValidateWithdrawalTxFromBurning(tx, storedVaultTx, spendingInputIdx, paramsFromVaultTxHeight); err != nil {
		if errors.Is(err, ErrInvalidWithdrawalTx) {
			// TODO consider slashing transaction for phase-2
			invalidTransactionsCounter.WithLabelValues("confirmed_withdraw_burning_transactions").Inc()
			si.logger.Warn("found an invalid withdrawal tx from burning",
				zap.String("tx_hash", tx.TxHash().String()),
				zap.Uint64("height", height),
				zap.Bool("is_confirmed", true),
				zap.Error(err),
			)

			return nil
		}

		failedProcessingWithdrawTxsFromBurningCounter.Inc()
		return err
	}

	burningTxHash := burningTx.Tx.TxHash()
	if err := si.processWithdrawTx(tx, burningTx.VaultTxHash, &burningTxHash, height); err != nil {
		// record metrics
		failedProcessingWithdrawTxsFromBurningCounter.Inc()
		return err
	}

	return nil
}

func (si *StakingIndexer) handleSpendingVaultTransaction(
	tx *wire.MsgTx,
	vaultTx *indexerstore.StoredVaultTransaction,
	spendingInputIndex int,
	height uint64,
	timestamp time.Time,
) error {
	vaultTxHash := vaultTx.Tx.TxHash()
	paramsFromVaultTxHeight, err := si.getVersionedParams(vaultTx.InclusionHeight)
	if err != nil {
		return err
	}

	// check whether it is a valid burning tx
	isBurning, err := si.IsValidBurningTx(tx, vaultTx, paramsFromVaultTxHeight)
	if err != nil {
		if errors.Is(err, ErrInvalidBurningTx) {
			invalidTransactionsCounter.WithLabelValues("confirmed_burning_transactions").Inc()
			si.logger.Warn("found an invalid burning tx",
				zap.String("tx_hash", tx.TxHash().String()),
				zap.Uint64("height", height),
				zap.Bool("is_confirmed", true),
				zap.Error(err),
			)

			return nil
		}
		// record metrics
		failedVerifyingBurningTxsCounter.Inc()
		return err
	}

	if !isBurning {
		// not an unbonding tx, so this is a slashingOrLostKey or burningWithourDApp tx from the vaultTx,
		// validate it and process it
		if err := si.ValidateWithdrawalTxFromVault(tx, vaultTx, spendingInputIndex, paramsFromVaultTxHeight); err != nil {
			if errors.Is(err, ErrInvalidWithdrawalTx) {
				invalidTransactionsCounter.WithLabelValues("confirmed_withdraw_vault_transactions").Inc()
				si.logger.Warn("found an invalid withdrawal tx from vault",
					zap.String("tx_hash", tx.TxHash().String()),
					zap.Uint64("height", height),
					zap.Bool("is_confirmed", true),
					zap.Error(err),
				)

				return nil
			}

			failedProcessingWithdrawTxsFromVaultCounter.Inc()
			return err
		}
		if err := si.processWithdrawVaultTx(tx, &vaultTxHash, nil, height); err != nil {
			// record metrics
			failedProcessingWithdrawTxsFromVaultCounter.Inc()

			return err
		}
		return nil
	}

	// 5. this is a valid burning tx, process it
	if err := si.ProcessBurningTx(
		tx, &vaultTxHash, height, timestamp,
		paramsFromVaultTxHeight,
	); err != nil {
		if !errors.Is(err, indexerstore.ErrDuplicateTransaction) {
			// record metrics
			failedProcessingBurningTxsCounter.Inc()

			return err
		}
		// we don't consider duplicate error critical as it can happen
		// when the indexer restarts
		si.logger.Warn("found a duplicate tx",
			zap.String("tx_hash", tx.TxHash().String()))
	}

	return nil
}

func (si *StakingIndexer) ValidateWithdrawalTxFromVault(
	tx *wire.MsgTx,
	vaultTx *indexerstore.StoredVaultTransaction,
	spendingInputIdx int,
	params *parser.ParsedVersionedGlobalParams,
) error {
	// re-build the time-lock path script and check whether the script from
	// the witness matches
	vaultInfo, err := btcvault.BuildVaultInfo(
		vaultTx.StakerPk,
		[]*btcec.PublicKey{vaultTx.DAppPk},
		params.CovenantPks,
		params.CovenantQuorum,
		btcutil.Amount(vaultTx.StakingValue),
		&si.cfg.BTCNetParams,
	)
	if err != nil {
		return fmt.Errorf("failed to rebuid the vault info: %w", err)
	}
	slashingOrLostKeyPathInfo, err := vaultInfo.SlashingOrLostKeyPathSpendInfo()
	if err != nil {
		return fmt.Errorf("failed to get the slashing or lost key path spend info: %w", err)
	}

	burnWithoutDAppPathInfo, err := vaultInfo.BurnWithoutDAppPathSpendInfo()
	if err != nil {
		return fmt.Errorf("failed to get the burn without dapp path spend info: %w", err)
	}

	witness := tx.TxIn[spendingInputIdx].Witness
	if len(witness) < 2 {
		panic(fmt.Errorf("spending tx should have at least 2 elements in witness, got %d", len(witness)))
	}

	scriptFromWitness := tx.TxIn[spendingInputIdx].Witness[len(tx.TxIn[spendingInputIdx].Witness)-2]

	checkSlashingOrLostKeyPath := bytes.Equal(slashingOrLostKeyPathInfo.GetPkScriptPath(), scriptFromWitness)
	checkBurnWithoutDAppPath := bytes.Equal(burnWithoutDAppPathInfo.GetPkScriptPath(), scriptFromWitness)

	if !checkSlashingOrLostKeyPath && !checkBurnWithoutDAppPath {
		return fmt.Errorf("%w: the tx does not unlock the slashing or lost key path and burn without dapp path", ErrInvalidWithdrawalTx)
	}

	return nil
}

func (si *StakingIndexer) ValidateWithdrawalTxFromBurning(
	tx *wire.MsgTx,
	vaultTx *indexerstore.StoredVaultTransaction,
	spendingInputIdx int,
	params *parser.ParsedVersionedGlobalParams,
) error {
	// re-build the time-lock path script and check whether the script from
	// the witness matches
	expectedBurningOutputValue := btcutil.Amount(vaultTx.StakingValue) - params.UnbondingFee
	burningInfo, err := btcvault.BuildBurningInfo(
		vaultTx.StakerPk,
		[]*btcec.PublicKey{vaultTx.DAppPk},
		params.CovenantPks,
		params.CovenantQuorum,
		expectedBurningOutputValue,
		&si.cfg.BTCNetParams,
	)
	if err != nil {
		return fmt.Errorf("failed to rebuid the burning info: %w", err)
	}
	burnWithoutDAppPathInfo, err := burningInfo.BurnWithoutDAppPathSpendInfo()
	if err != nil {
		return fmt.Errorf("failed to get the burning path spend info: %w", err)
	}

	witness := tx.TxIn[spendingInputIdx].Witness
	if len(witness) < 2 {
		panic(fmt.Errorf("spending tx should have at least 2 elements in witness, got %d", len(witness)))
	}

	scriptFromWitness := tx.TxIn[spendingInputIdx].Witness[len(tx.TxIn[spendingInputIdx].Witness)-2]

	if !bytes.Equal(burnWithoutDAppPathInfo.GetPkScriptPath(), scriptFromWitness) {
		return fmt.Errorf("%w: the tx does not unlock the burn without dApp path", ErrInvalidWithdrawalTx)
	}

	return nil
}

// ///////
func (si *StakingIndexer) getSpentVaultTxs(tx *wire.MsgTx) ([]*indexerstore.StoredVaultTransaction, []int) {
	storedVaultTxs := make([]*indexerstore.StoredVaultTransaction, 0)
	spendingInputIndexes := make([]int, 0)
	for i, txIn := range tx.TxIn {
		maybeStakingTxHash := txIn.PreviousOutPoint.Hash
		vaultTx, err := si.GetVaultTxByHash(&maybeStakingTxHash)
		if err != nil || vaultTx == nil {
			continue
		}

		// this ensures the spending tx spends the correct staking output
		if txIn.PreviousOutPoint.Index != vaultTx.StakingOutputIdx {
			continue
		}

		storedVaultTxs = append(storedVaultTxs, vaultTx)
		spendingInputIndexes = append(spendingInputIndexes, i)
	}

	return storedVaultTxs, spendingInputIndexes
}

func getSpentFromVaultTxs(
	tx *wire.MsgTx,
	stakingTxs map[chainhash.Hash]*indexerstore.StoredVaultTransaction,
) ([]*indexerstore.StoredVaultTransaction, []int) {
	storedVaultTxs := make([]*indexerstore.StoredVaultTransaction, 0)
	spendingInputIndexes := make([]int, 0)
	for i, txIn := range tx.TxIn {
		maybeVaultTxHash := txIn.PreviousOutPoint.Hash
		stakingTx, exists := stakingTxs[maybeVaultTxHash]
		if !exists {
			continue
		}

		// this ensures the spending tx spends the correct staking output
		if txIn.PreviousOutPoint.Index != stakingTx.StakingOutputIdx {
			continue
		}

		storedVaultTxs = append(storedVaultTxs, stakingTx)
		spendingInputIndexes = append(spendingInputIndexes, i)
	}

	return storedVaultTxs, spendingInputIndexes
}

func (si *StakingIndexer) getSpentBurningTxs(tx *wire.MsgTx) ([]*indexerstore.StoredBurningTransaction, []int) {
	storedBurningTxs := make([]*indexerstore.StoredBurningTransaction, 0)
	spendingInputIndexes := make([]int, 0)
	for i, txIn := range tx.TxIn {
		maybeUnbondingTxHash := txIn.PreviousOutPoint.Hash
		burningTx, err := si.GetBurningTxByHash(&maybeUnbondingTxHash)
		if err != nil || burningTx == nil {
			continue
		}

		storedBurningTxs = append(storedBurningTxs, burningTx)
		spendingInputIndexes = append(spendingInputIndexes, i)
	}

	return storedBurningTxs, spendingInputIndexes
}

func (si *StakingIndexer) IsValidBurningTx(tx *wire.MsgTx, vaultTx *indexerstore.StoredVaultTransaction, params *parser.ParsedVersionedGlobalParams) (bool, error) {
	// 1. an unbonding tx must be a transfer tx
	if err := btcstaking.IsTransferTx(tx); err != nil {
		return false, nil
	}

	// 2. an unbonding tx must spend the staking output
	vaultTxHash := vaultTx.Tx.TxHash()
	if !tx.TxIn[0].PreviousOutPoint.Hash.IsEqual(&vaultTxHash) {
		return false, nil
	}
	if tx.TxIn[0].PreviousOutPoint.Index != vaultTx.StakingOutputIdx {
		return false, nil
	}

	// 3. re-build the unbonding path script and check whether the script from
	// the witness matches
	vaultInfo, err := btcvault.BuildVaultInfo(
		vaultTx.StakerPk,
		[]*btcec.PublicKey{vaultTx.DAppPk},
		params.CovenantPks,
		params.CovenantQuorum,
		btcutil.Amount(vaultTx.StakingValue),
		&si.cfg.BTCNetParams,
	)
	if err != nil {
		return false, fmt.Errorf("failed to rebuid the vault info: %w", err)
	}
	burningPathInfo, err := vaultInfo.BurnPathSpendInfo()
	if err != nil {
		return false, fmt.Errorf("failed to get the burning path spend info: %w", err)
	}

	witness := tx.TxIn[0].Witness
	if len(witness) < 2 {
		panic(fmt.Errorf("spending tx should have at least 2 elements in witness, got %d", len(witness)))
	}

	scriptFromWitness := tx.TxIn[0].Witness[len(tx.TxIn[0].Witness)-2]

	if !bytes.Equal(burningPathInfo.GetPkScriptPath(), scriptFromWitness) {
		// not burning tx as it does not unlock the burning path
		return false, nil
	}

	// 4. check whether the unbonding tx enables rbf has time lock
	if tx.TxIn[0].Sequence != wire.MaxTxInSequenceNum {
		return false, fmt.Errorf("%w: burning tx should not enable rbf", ErrInvalidBurningTx)
	}

	if tx.LockTime != 0 {
		return false, fmt.Errorf("%w: burning tx should not set lock time", ErrInvalidBurningTx)
	}

	// 5. check whether the script of an unbonding tx output is expected
	// by re-building unbonding output from params
	vaultValue := btcutil.Amount(vaultTx.Tx.TxOut[vaultTx.StakingOutputIdx].Value)
	expectedBurningOutputValue := vaultValue - params.UnbondingFee
	if expectedBurningOutputValue <= 0 {
		return false, fmt.Errorf("%w: vault output value is too low, got %v, vault fee: %v",
			ErrInvalidBurningTx, vaultValue, params.UnbondingFee)
	}
	burningInfo, err := btcvault.BuildBurningInfo(
		vaultTx.StakerPk,
		[]*btcec.PublicKey{vaultTx.DAppPk},
		params.CovenantPks,
		params.CovenantQuorum,
		expectedBurningOutputValue,
		&si.cfg.BTCNetParams,
	)
	if err != nil {
		return false, fmt.Errorf("failed to rebuid the burning info: %w", err)
	}
	if !bytes.Equal(tx.TxOut[0].PkScript, burningInfo.BurningOutput.PkScript) {
		return false, fmt.Errorf("%w: the burning output is not expected", ErrInvalidBurningTx)
	}
	if tx.TxOut[0].Value != burningInfo.BurningOutput.Value {
		return false, fmt.Errorf("%w: the burning output value %d is not expected %d",
			ErrInvalidBurningTx, tx.TxOut[0].Value, burningInfo.BurningOutput.Value)
	}

	return true, nil
}

func (si *StakingIndexer) ProcessVaultTx(
	tx *wire.MsgTx,
	vaultData *btcvault.ParsedV0VaultTx,
	height uint64, timestamp time.Time,
	params *parser.ParsedVersionedGlobalParams,
) error {
	var (
		// whether the staking tx is overflow
		isOverflow bool
	)

	si.logger.Info("found a vault tx",
		zap.Uint64("height", height),
		zap.String("tx_hash", tx.TxHash().String()),
		zap.Int64("value", vaultData.VaultOutput.Value),
	)

	// check whether the staking tx already exists in db
	// if so, get the isOverflow from the data in db
	// otherwise, check it if the current tvl already reaches
	// the cap
	txHash := tx.TxHash()
	storedVaultTx, err := si.is.GetVaultTransaction(&txHash)
	if err != nil {
		return err
	}
	if storedVaultTx != nil {
		isOverflow = storedVaultTx.IsOverflow
	} else {
		// this is a new vault tx, validate it against vault requirement
		if err := si.validateVaultTx(params, vaultData); err != nil {
			invalidTransactionsCounter.WithLabelValues("confirmed_vault_transaction").Inc()
			si.logger.Warn("found an invalid vault tx",
				zap.String("tx_hash", tx.TxHash().String()),
				zap.Uint64("height", height),
				zap.Bool("is_confirmed", true),
				zap.Error(err),
			)
			// TODO handle invalid vault tx (storing and pushing events)
			return nil
		}

		// check if the vault tvl is overflow with this vault tx
		vaultOverflow, err := si.isOverflow(height, params)
		if err != nil {
			return fmt.Errorf("failed to check the overflow of vault tx: %w", err)
		}

		isOverflow = vaultOverflow
	}

	if isOverflow {
		si.logger.Info("the vault tx is overflow",
			zap.String("tx_hash", tx.TxHash().String()))
	}

	// add the staking transaction to the system state
	if err := si.addVaultTransaction(
		height, timestamp, tx,
		vaultData.OpReturnData.StakerPublicKey.PubKey,
		vaultData.OpReturnData.FinalityProviderPublicKey.PubKey,
		uint64(vaultData.VaultOutput.Value),
		uint32(vaultData.VaultOutputIdx),
		vaultData.PayloadOpReturnData.ChainID,
		vaultData.PayloadOpReturnData.ChainIdUserAddress,
		vaultData.PayloadOpReturnData.ChainIdSmartContractAddress,
		vaultData.PayloadOpReturnData.Amount,
		isOverflow,
	); err != nil {
		return err
	}
	return nil
}

func (si *StakingIndexer) addVaultTransaction(
	height uint64,
	timestamp time.Time,
	tx *wire.MsgTx,
	stakerPk *btcec.PublicKey,
	dAppPk *btcec.PublicKey,
	stakingValue uint64,
	stakingOutputIndex uint32,
	chainID []byte,
	chainIdUserAddress []byte,
	chainIdSmartContractAddress []byte,
	amountMinting []byte,
	isOverflow bool,
) error {
	txHex, err := getTxHex(tx)
	if err != nil {
		return err
	}

	vaultEvent := queuecli.NewActiveVaultEvent(
		tx.TxHash().String(),
		txHex,
		hex.EncodeToString(schnorr.SerializePubKey(stakerPk)),
		hex.EncodeToString(schnorr.SerializePubKey(dAppPk)),
		stakingValue,
		height,
		timestamp.Unix(),
		uint64(stakingOutputIndex),
		chainID,
		chainIdUserAddress,
		chainIdSmartContractAddress,
		amountMinting,
		isOverflow,
	)

	// push the events first then save the tx due to the assumption
	// that the consumer can handle duplicate events
	if err := si.consumer.PushVaultEvent(&vaultEvent); err != nil {
		return fmt.Errorf("failed to push the vault event to the queue: %w", err)
	}

	si.logger.Info("saving the vault transaction",
		zap.String("tx_hash", tx.TxHash().String()),
	)
	// save the staking tx in the db
	if err := si.is.AddVaultTransaction(
		tx, stakingOutputIndex, height,
		stakerPk, dAppPk,
		stakingValue, chainID, chainIdUserAddress, chainIdSmartContractAddress, amountMinting, isOverflow,
	); err != nil && !errors.Is(err, indexerstore.ErrDuplicateTransaction) {
		return fmt.Errorf("failed to add the vault tx to store: %w", err)
	}

	si.logger.Info("successfully saved the vault transaction",
		zap.String("tx_hash", tx.TxHash().String()),
	)

	// record metrics
	if isOverflow {
		totalVaultTxs.WithLabelValues("overflow").Inc()
	} else {
		totalVaultTxs.WithLabelValues("active").Inc()
	}
	lastFoundVaultTxHeight.Set(float64(height))
	return nil
}

func (si *StakingIndexer) ProcessBurningTx(
	tx *wire.MsgTx,
	vaultTxHash *chainhash.Hash,
	height uint64, timestamp time.Time,
	params *parser.ParsedVersionedGlobalParams,
) error {
	si.logger.Info("found an burning tx",
		zap.Uint64("height", height),
		zap.String("tx_hash", tx.TxHash().String()),
		zap.String("staking_tx_hash", vaultTxHash.String()),
	)

	burningTxHex, err := getTxHex(tx)
	if err != nil {
		return err
	}

	burningTxHash := tx.TxHash()
	burningEvent := queuecli.NewBurningVaultEvent(
		burningTxHash.String(),
		height,
		timestamp.Unix(),
		// valid burning tx always has one output
		0,
		burningTxHex,
		burningTxHash.String(),
	)

	if err := si.consumer.PushBurningEvent(&burningEvent); err != nil {
		return fmt.Errorf("failed to push the burning event to the queue: %w", err)
	}

	si.logger.Info("saving the burning tx",
		zap.String("tx_hash", burningTxHash.String()))

	if err := si.is.AddBurningTransaction(
		tx,
		vaultTxHash,
	); err != nil && !errors.Is(err, indexerstore.ErrDuplicateTransaction) {
		return fmt.Errorf("failed to add the burning tx to store: %w", err)
	}

	si.logger.Info("successfully saved the burning tx",
		zap.String("tx_hash", tx.TxHash().String()))

	// record metrics
	totalBurningTxs.Inc()
	lastFoundBurningTxHeight.Set(float64(height))

	return nil
}

func (si *StakingIndexer) processWithdrawVaultTx(tx *wire.MsgTx, vaultTxHash *chainhash.Hash, burningTxHash *chainhash.Hash, height uint64) error {
	txHashHex := tx.TxHash().String()
	if burningTxHash == nil {
		si.logger.Info("found a withdraw tx from vault",
			zap.String("tx_hash", txHashHex),
			zap.String("vault_tx_hash", vaultTxHash.String()),
		)
	} else {
		si.logger.Info("found a withdraw tx from burning",
			zap.String("tx_hash", txHashHex),
			zap.String("vault_tx_hash", vaultTxHash.String()),
			zap.String("unbonding_tx_hash", burningTxHash.String()),
		)
	}

	withdrawEvent := queuecli.NewWithdrawVaultEvent(vaultTxHash.String())

	if err := si.consumer.PushWithdrawVaultEvent(&withdrawEvent); err != nil {
		return fmt.Errorf("failed to push the withdraw event to the consumer: %w", err)
	}

	// record metrics
	if burningTxHash == nil {
		totalWithdrawTxsFromVault.Inc()
		lastFoundWithdrawTxFromVaultHeight.Set(float64(height))
	} else {
		totalWithdrawTxsFromBurning.Inc()
		lastFoundWithdrawTxFromBurningHeight.Set(float64(height))
	}

	return nil
}

func (si *StakingIndexer) tryParseVaultTx(tx *wire.MsgTx, params *parser.ParsedVersionedGlobalParams) (*btcvault.ParsedV0VaultTx, error) {
	possible := btcvault.IsPossibleV0VaultTx(tx, params.Tag)
	if !possible {
		return nil, fmt.Errorf("not staking tx")
	}
	parsedData, err := btcvault.ParseV0VaultTx(
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

func (si *StakingIndexer) GetVaultTxByHash(hash *chainhash.Hash) (*indexerstore.StoredVaultTransaction, error) {
	return si.is.GetVaultTransaction(hash)
}

func (si *StakingIndexer) GetBurningTxByHash(hash *chainhash.Hash) (*indexerstore.StoredBurningTransaction, error) {
	return si.is.GetBurningTransaction(hash)
}

func (si *StakingIndexer) validateVaultTx(params *parser.ParsedVersionedGlobalParams, vaultData *btcvault.ParsedV0VaultTx) error {
	value := btcutil.Amount(vaultData.VaultOutput.Value)
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
	return nil
}

package indexerstore

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/kvdb"
	pm "google.golang.org/protobuf/proto"

	"github.com/babylonchain/staking-indexer/proto"
	"github.com/babylonchain/staking-indexer/utils"
)

var (
	// mapping tx hash -> Transaction
	transactionBucketName = []byte("transactions")
)

type IndexerStore struct {
	db kvdb.Backend
}

type StoredStakingTransaction struct {
	Tx                 *wire.MsgTx
	StakingOutputIdx   uint32
	InclusionHeight    uint64
	StakerPk           *btcec.PublicKey
	StakingTime        uint32
	FinalityProviderPk *btcec.PublicKey
}

// NewIndexerStore returns a new store backed by db
func NewIndexerStore(db kvdb.Backend) (*IndexerStore,
	error) {

	store := &IndexerStore{db}
	if err := store.initBuckets(); err != nil {
		return nil, err
	}

	return store, nil
}

func (c *IndexerStore) initBuckets() error {
	return kvdb.Batch(c.db, func(tx kvdb.RwTx) error {
		_, err := tx.CreateTopLevelBucket(transactionBucketName)
		if err != nil {
			return err
		}

		return nil
	})
}

func (is *IndexerStore) AddStakingTransaction(
	tx *wire.MsgTx,
	stakingOutputIdx uint32,
	inclusionHeight uint64,
	stakerPk *btcec.PublicKey,
	stakingTime uint32,
	fpPk *btcec.PublicKey,
) error {
	txHash := tx.TxHash()
	serializedTx, err := utils.SerializeBtcTransaction(tx)

	if err != nil {
		return err
	}

	msg := proto.StakingTransaction{
		TransactionBytes:   serializedTx,
		StakingOutputIdx:   stakingOutputIdx,
		InclusionHeight:    inclusionHeight,
		StakingTime:        stakingTime,
		StakerPk:           schnorr.SerializePubKey(stakerPk),
		FinalityProviderPk: schnorr.SerializePubKey(fpPk),
	}

	return is.addStakingTransaction(txHash[:], &msg)
}

func (is *IndexerStore) addStakingTransaction(
	txHashBytes []byte,
	st *proto.StakingTransaction,
) error {
	return kvdb.Batch(is.db, func(tx kvdb.RwTx) error {

		txBucket := tx.ReadWriteBucket(transactionBucketName)
		if txBucket == nil {
			return ErrCorruptedTransactionsDb
		}
		maybeTx := txBucket.Get(txHashBytes)
		if maybeTx != nil {
			return ErrDuplicateTransaction
		}

		marshalled, err := pm.Marshal(st)
		if err != nil {
			return err
		}

		return txBucket.Put(txHashBytes, marshalled)
	})
}

func (is *IndexerStore) GetTransaction(txHash *chainhash.Hash) (*StoredStakingTransaction, error) {
	var storedTx *StoredStakingTransaction
	txHashBytes := txHash.CloneBytes()

	err := is.db.View(func(tx kvdb.RTx) error {
		txBucket := tx.ReadBucket(transactionBucketName)
		if txBucket == nil {
			return ErrCorruptedTransactionsDb
		}

		maybeTx := txBucket.Get(txHashBytes)
		if maybeTx == nil {
			return ErrTransactionNotFound
		}

		var storedTxProto proto.StakingTransaction
		if err := pm.Unmarshal(maybeTx, &storedTxProto); err != nil {
			return ErrCorruptedTransactionsDb
		}

		txFromDb, err := protoStakingTxToStoredStakingTx(&storedTxProto)
		if err != nil {
			return err
		}

		storedTx = txFromDb
		return nil
	}, func() {})

	if err != nil {
		return nil, err
	}

	return storedTx, nil
}

func protoStakingTxToStoredStakingTx(protoTx *proto.StakingTransaction) (*StoredStakingTransaction, error) {
	var stakingTx wire.MsgTx
	err := stakingTx.Deserialize(bytes.NewReader(protoTx.TransactionBytes))
	if err != nil {
		return nil, fmt.Errorf("invalid staking tx: %w", err)
	}

	stakerPk, err := schnorr.ParsePubKey(protoTx.StakerPk)
	if err != nil {
		return nil, fmt.Errorf("invalid staker pk: %w", err)
	}

	fpPk, err := schnorr.ParsePubKey(protoTx.FinalityProviderPk)
	if err != nil {
		return nil, fmt.Errorf("invalid finality provider pk: %w", err)
	}

	return &StoredStakingTransaction{
		Tx:                 &stakingTx,
		StakingOutputIdx:   protoTx.StakingOutputIdx,
		InclusionHeight:    protoTx.InclusionHeight,
		StakerPk:           stakerPk,
		StakingTime:        protoTx.StakingTime,
		FinalityProviderPk: fpPk,
	}, nil
}

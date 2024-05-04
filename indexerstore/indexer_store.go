package indexerstore

import (
	"bytes"
	"encoding/binary"
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
	// mapping tx hash -> staking transaction
	stakingTxBucketName = []byte("stakingtxs")

	// mapping tx hash -> unbonding transaction
	unbondingTxBucketName = []byte("unbondingtxs")

	// stores indexer state
	indexerStateBucketName = []byte("indexerstate")

	// stores the confirmed tvl
	confirmedTvlBucketName = []byte("confirmedtvl")
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
	IsOverflow         bool
	StakingValue       int64
}

type StoredUnbondingTransaction struct {
	Tx            *wire.MsgTx
	StakingTxHash *chainhash.Hash
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
		_, err := tx.CreateTopLevelBucket(stakingTxBucketName)
		if err != nil {
			return err
		}

		_, err = tx.CreateTopLevelBucket(unbondingTxBucketName)
		if err != nil {
			return err
		}

		_, err = tx.CreateTopLevelBucket(indexerStateBucketName)
		if err != nil {
			return err
		}

		_, err = tx.CreateTopLevelBucket(confirmedTvlBucketName)
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
	stakingValue int64,
	isOverflow bool,
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
		IsOverflow:         isOverflow,
		StakingValue:       stakingValue,
	}

	return is.addStakingTransaction(txHash[:], &msg)
}

func (is *IndexerStore) addStakingTransaction(
	txHashBytes []byte,
	st *proto.StakingTransaction,
) error {
	return kvdb.Batch(is.db, func(tx kvdb.RwTx) error {

		txBucket := tx.ReadWriteBucket(stakingTxBucketName)
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

		err = txBucket.Put(txHashBytes, marshalled)
		if err != nil {
			return err
		}

		// if the staking tx is an overflow, we don't increment the confirmed tvl
		if st.IsOverflow {
			return nil
		}
		return is.incrementConfirmedTvl(tx, st.StakingValue)
	})
}

func (is *IndexerStore) GetStakingTransaction(txHash *chainhash.Hash) (*StoredStakingTransaction, error) {
	var storedTx *StoredStakingTransaction
	txHashBytes := txHash.CloneBytes()

	err := is.db.View(func(tx kvdb.RTx) error {
		txBucket := tx.ReadBucket(stakingTxBucketName)
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
		IsOverflow:         protoTx.IsOverflow,
		StakingValue:       protoTx.StakingValue,
	}, nil
}

func (is *IndexerStore) AddUnbondingTransaction(
	tx *wire.MsgTx,
	stakingTxHash *chainhash.Hash,
) error {
	txHash := tx.TxHash()
	serializedTx, err := utils.SerializeBtcTransaction(tx)

	if err != nil {
		return err
	}

	stakingTxHashBytes := stakingTxHash.CloneBytes()
	msg := proto.UnbondingTransaction{
		TransactionBytes: serializedTx,
		StakingTxHash:    stakingTxHash.CloneBytes(),
	}

	return is.addUnbondingTransaction(txHash[:], stakingTxHashBytes, &msg)
}

func (is *IndexerStore) addUnbondingTransaction(
	txHashBytes []byte,
	stakingHashBytes []byte,
	ut *proto.UnbondingTransaction,
) error {
	return kvdb.Batch(is.db, func(tx kvdb.RwTx) error {
		stakingTxBucket := tx.ReadWriteBucket(stakingTxBucketName)
		if stakingTxBucket == nil {
			return ErrCorruptedTransactionsDb
		}

		// we need to ensure the staking tx already exists
		maybeStakingTx := stakingTxBucket.Get(stakingHashBytes)
		if maybeStakingTx == nil {
			return ErrTransactionNotFound
		}
		// parse it, make sure it's valid
		var storedTxProto proto.StakingTransaction
		if err := pm.Unmarshal(maybeStakingTx, &storedTxProto); err != nil {
			return ErrCorruptedTransactionsDb
		}

		unbondingTxBucket := tx.ReadWriteBucket(unbondingTxBucketName)
		if unbondingTxBucket == nil {
			return ErrCorruptedTransactionsDb
		}

		// check duplicate
		maybeTx := unbondingTxBucket.Get(txHashBytes)
		if maybeTx != nil {
			return ErrDuplicateTransaction
		}

		marshalled, err := pm.Marshal(ut)
		if err != nil {
			return err
		}

		err = unbondingTxBucket.Put(txHashBytes, marshalled)
		if err != nil {
			return err
		}

		// if the staking tx is an overflow, we don't decrement the confirmed tvl
		// as it was never added
		if storedTxProto.IsOverflow {
			return nil
		}

		return is.subtractConfirmedTvl(
			tx, storedTxProto.StakingValue,
		)
	})
}

func (is *IndexerStore) GetUnbondingTransaction(txHash *chainhash.Hash) (*StoredUnbondingTransaction, error) {
	var storedTx *StoredUnbondingTransaction
	txHashBytes := txHash.CloneBytes()

	err := is.db.View(func(tx kvdb.RTx) error {
		txBucket := tx.ReadBucket(unbondingTxBucketName)
		if txBucket == nil {
			return ErrCorruptedTransactionsDb
		}

		maybeTx := txBucket.Get(txHashBytes)
		if maybeTx == nil {
			return ErrTransactionNotFound
		}

		var storedTxProto proto.UnbondingTransaction
		if err := pm.Unmarshal(maybeTx, &storedTxProto); err != nil {
			return ErrCorruptedTransactionsDb
		}

		txFromDb, err := protoUnbondingTxToStoredUnbondingTx(&storedTxProto)
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

func protoUnbondingTxToStoredUnbondingTx(protoTx *proto.UnbondingTransaction) (*StoredUnbondingTransaction, error) {
	var unbondingTx wire.MsgTx
	err := unbondingTx.Deserialize(bytes.NewReader(protoTx.TransactionBytes))
	if err != nil {
		return nil, fmt.Errorf("invalid unbonding tx: %w", err)
	}

	stakingTxHash, err := chainhash.NewHash(protoTx.StakingTxHash)
	if err != nil {
		return nil, fmt.Errorf("invalid staking tx hash")
	}

	return &StoredUnbondingTransaction{
		Tx:            &unbondingTx,
		StakingTxHash: stakingTxHash,
	}, nil
}

func getConfirmedTvlKey() []byte {
	return []byte("confirmedtvl")
}

// incrementConfirmedTvl increments the confirmed tvl
func (is *IndexerStore) incrementConfirmedTvl(
	tx kvdb.RwTx, tvlIncrement int64,
) error {
	tvlBucket := tx.ReadWriteBucket(confirmedTvlBucketName)
	key := getConfirmedTvlKey()
	if tvlBucket == nil {
		return ErrCorruptedStateDb
	}

	currentTvl := tvlBucket.Get(key)
	var confirmedTvl int64
	if currentTvl != nil {
		var err error
		confirmedTvl, err = int64FromBytes(currentTvl)
		if err != nil {
			return err
		}
	}

	newTvl := confirmedTvl + tvlIncrement
	newTvlBytes := int64ToBytes(newTvl)

	return tvlBucket.Put(key, newTvlBytes)
}

// SubtractConfirmedTvl subtracts the confirmed tvl
func (is *IndexerStore) subtractConfirmedTvl(
	tx kvdb.RwTx, tvlSubtract int64,
) error {
	key := getConfirmedTvlKey()
	tvlBucket := tx.ReadWriteBucket(confirmedTvlBucketName)
	if tvlBucket == nil {
		return ErrCorruptedStateDb
	}

	currentTvl := tvlBucket.Get(key)
	if currentTvl == nil {
		// This should never happen, return an error
		return ErrCorruptedStateDb
	}
	confirmedTvl, err := int64FromBytes(currentTvl)
	if err != nil {
		return err
	}

	if tvlSubtract > confirmedTvl {
		return ErrNegativeTvl
	}

	newTvlBytes := int64ToBytes(confirmedTvl - tvlSubtract)

	return tvlBucket.Put(key, newTvlBytes)
}

// GetConfirmedTvl returns the confirmed tvl
func (is *IndexerStore) GetConfirmedTvl() (int64, error) {
	key := getConfirmedTvlKey()

	var confirmedTvl int64
	err := is.db.View(func(tx kvdb.RTx) error {
		tvlBucket := tx.ReadBucket(confirmedTvlBucketName)
		if tvlBucket == nil {
			return ErrCorruptedStateDb
		}

		v := tvlBucket.Get(key)
		if v == nil {
			// This could happen if the indexer is started for the first time
			confirmedTvl = 0
			return nil
		}

		tvl, err := int64FromBytes(v)
		if err != nil {
			return err
		}

		confirmedTvl = tvl

		return nil
	}, func() {})

	if err != nil {
		return 0, err
	}

	return confirmedTvl, nil
}

func getLastProcessedHeightKey() []byte {
	return []byte("lastprocessedheight")
}

func (is *IndexerStore) SaveLastProcessedHeight(height uint64) error {
	key := getLastProcessedHeightKey()
	heightBytes := uint64ToBytes(height)

	return kvdb.Batch(is.db, func(tx kvdb.RwTx) error {
		stateBucket := tx.ReadWriteBucket(indexerStateBucketName)
		if stateBucket == nil {
			return ErrCorruptedStateDb
		}

		return stateBucket.Put(key, heightBytes)
	})
}

func (is *IndexerStore) GetLastProcessedHeight() (uint64, error) {
	key := getLastProcessedHeightKey()

	var lastProcessedHeight uint64

	err := is.db.View(func(tx kvdb.RTx) error {
		stateBucket := tx.ReadBucket(indexerStateBucketName)
		if stateBucket == nil {
			return ErrCorruptedStateDb
		}

		v := stateBucket.Get(key)
		if v == nil {
			return ErrLastProcessedHeightNotFound
		}

		height, err := uint64FromBytes(v)
		if err != nil {
			return err
		}

		lastProcessedHeight = height

		return nil
	}, func() {})

	if err != nil {
		return 0, err
	}

	return lastProcessedHeight, nil
}

func uint64ToBytes(v uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	return buf[:]
}

func uint64FromBytes(b []byte) (uint64, error) {
	if len(b) != 8 {
		return 0, fmt.Errorf("invalid uint64 bytes length: %d", len(b))
	}

	return binary.BigEndian.Uint64(b), nil
}

func int64ToBytes(v int64) []byte {
	buf := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		buf[i] = byte(v)
		v >>= 8
	}
	return buf
}

func int64FromBytes(buf []byte) (int64, error) {
	if len(buf) != 8 {
		return 0, fmt.Errorf("invalid int64 bytes length: %d", len(buf))
	}
	var result int64
	for _, b := range buf {
		result <<= 8
		result |= int64(b)
	}
	return result, nil
}

package indexerstore

import (
	"bytes"
	"errors"
	"fmt"

	// "github.com/babylonchain/staking-indexer/proto"
	"github.com/babylonchain/staking-indexer/utils"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/scalarorg/staking-indexer/proto"
	pm "google.golang.org/protobuf/proto"
)

var (
	// mapping tx hash -> staking transaction
	vaultTxBucketName = []byte("vaulttxs")

	// mapping tx hash -> unbonding transaction
	burningTxBucketName = []byte("burningtxs")
)

type StoredVaultTransaction struct {
	Tx                          *wire.MsgTx
	StakingOutputIdx            uint32
	InclusionHeight             uint64
	StakerPk                    *btcec.PublicKey
	StakingValue                uint64
	DAppPk                      *btcec.PublicKey
	ChainID                     []byte
	ChainIdUserAddress          []byte
	ChainIdSmartContractAddress []byte
	MintingAmount               []byte
	IsOverflow                  bool
}

type StoredBurningTransaction struct {
	Tx          *wire.MsgTx
	VaultTxHash *chainhash.Hash
}

func (is *IndexerStore) AddVaultTransaction(
	tx *wire.MsgTx,
	stakingOutputIdx uint32,
	inclusionHeight uint64,
	stakerPk *btcec.PublicKey,
	dAppPk *btcec.PublicKey,
	stakingValue uint64,
	chainID []byte,
	chainIdUserAddress []byte,
	chainIdSmartContractAddress []byte,
	amountMinting []byte,
	isOverflow bool,
) error {
	txHash := tx.TxHash()
	serializedTx, err := utils.SerializeBtcTransaction(tx)
	if err != nil {
		return err
	}

	msg := proto.VaultTransaction{
		TransactionBytes:            serializedTx,
		StakingOutputIdx:            stakingOutputIdx,
		InclusionHeight:             inclusionHeight,
		StakerPk:                    schnorr.SerializePubKey(stakerPk),
		FinalityProviderPk:          schnorr.SerializePubKey(dAppPk),
		StakingValue:                stakingValue,
		ChainID:                     chainID,
		ChainIdUserAddress:          chainIdUserAddress,
		ChainIdSmartContractAddress: chainIdSmartContractAddress,
		MintingAmount:               amountMinting,
		IsOverflow:                  isOverflow,
	}
	return is.addVaultTransaction(txHash[:], &msg)
}

func (is *IndexerStore) addVaultTransaction(
	txHashBytes []byte,
	st *proto.VaultTransaction,
) error {
	return kvdb.Batch(is.db, func(tx kvdb.RwTx) error {

		txBucket := tx.ReadWriteBucket(vaultTxBucketName)
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

		// if the vault tx is an overflow, we don't increment the confirmed tvl
		if st.IsOverflow {
			return nil
		}
		return is.incrementConfirmedTvl(tx, st.StakingValue)
	})
}

func (is *IndexerStore) GetVaultTransaction(txHash *chainhash.Hash) (*StoredVaultTransaction, error) {
	var storedTx *StoredVaultTransaction
	txHashBytes := txHash.CloneBytes()

	err := is.db.View(func(tx kvdb.RTx) error {
		txBucket := tx.ReadBucket(vaultTxBucketName)
		if txBucket == nil {
			return ErrCorruptedTransactionsDb
		}

		maybeTx := txBucket.Get(txHashBytes)
		if maybeTx == nil {
			return ErrTransactionNotFound
		}

		var storedTxProto proto.VaultTransaction
		if err := pm.Unmarshal(maybeTx, &storedTxProto); err != nil {
			return ErrCorruptedTransactionsDb
		}

		txFromDb, err := protoVaultTxToStoredVaultTx(&storedTxProto)
		if err != nil {
			return err
		}

		storedTx = txFromDb
		return nil
	}, func() {})

	if err != nil && !errors.Is(err, ErrTransactionNotFound) {
		return nil, err
	}

	return storedTx, nil
}

func protoVaultTxToStoredVaultTx(protoTx *proto.VaultTransaction) (*StoredVaultTransaction, error) {
	var vaultTx wire.MsgTx
	err := vaultTx.Deserialize(bytes.NewReader(protoTx.TransactionBytes))
	if err != nil {
		return nil, fmt.Errorf("invalid staking tx: %w", err)
	}

	stakerPk, err := schnorr.ParsePubKey(protoTx.StakerPk)
	if err != nil {
		return nil, fmt.Errorf("invalid staker pk: %w", err)
	}

	dAppPk, err := schnorr.ParsePubKey(protoTx.FinalityProviderPk)
	if err != nil {
		return nil, fmt.Errorf("invalid finality provider pk: %w", err)
	}

	return &StoredVaultTransaction{
		Tx:                          &vaultTx,
		StakingOutputIdx:            protoTx.StakingOutputIdx,
		InclusionHeight:             protoTx.InclusionHeight,
		StakerPk:                    stakerPk,
		StakingValue:                protoTx.StakingValue,
		DAppPk:                      dAppPk,
		ChainID:                     protoTx.ChainID,
		ChainIdUserAddress:          protoTx.ChainIdUserAddress,
		ChainIdSmartContractAddress: protoTx.ChainIdSmartContractAddress,
		MintingAmount:               protoTx.MintingAmount,
		IsOverflow:                  protoTx.IsOverflow,
	}, nil
}

func (is *IndexerStore) AddBurningTransaction(
	tx *wire.MsgTx,
	vaultTxHash *chainhash.Hash,
) error {
	txHash := tx.TxHash()
	serializedTx, err := utils.SerializeBtcTransaction(tx)

	if err != nil {
		return err
	}

	vaultTxHashBytes := vaultTxHash.CloneBytes()
	msg := proto.BurningTransaction{
		TransactionBytes: serializedTx,
		VaultTxHash:      vaultTxHash.CloneBytes(),
	}

	return is.addBurningTransaction(txHash[:], vaultTxHashBytes, &msg)
}

func (is *IndexerStore) addBurningTransaction(
	txHashBytes []byte,
	vaultHashBytes []byte,
	ut *proto.BurningTransaction,
) error {
	return kvdb.Batch(is.db, func(tx kvdb.RwTx) error {
		vaultTxBucket := tx.ReadWriteBucket(vaultTxBucketName)
		if vaultTxBucket == nil {
			return ErrCorruptedTransactionsDb
		}

		// we need to ensure the staking tx already exists
		maybeVaultTx := vaultTxBucket.Get(vaultHashBytes)
		if maybeVaultTx == nil {
			return ErrTransactionNotFound
		}
		// parse it, make sure it's valid
		var storedTxProto proto.VaultTransaction
		if err := pm.Unmarshal(maybeVaultTx, &storedTxProto); err != nil {
			return ErrCorruptedTransactionsDb
		}

		burningTxBucket := tx.ReadWriteBucket(burningTxBucketName)
		if burningTxBucket == nil {
			return ErrCorruptedTransactionsDb
		}

		// check duplicate
		maybeTx := burningTxBucket.Get(txHashBytes)
		if maybeTx != nil {
			return ErrDuplicateTransaction
		}

		marshalled, err := pm.Marshal(ut)
		if err != nil {
			return err
		}

		err = burningTxBucket.Put(txHashBytes, marshalled)
		if err != nil {
			return err
		}

		// if the vault tx is an overflow, we don't decrement the confirmed tvl
		// as it was never added
		if storedTxProto.IsOverflow {
			return nil
		}

		return is.subtractConfirmedTvl(
			tx, storedTxProto.StakingValue,
		)
	})
}

func (is *IndexerStore) GetBurningTransaction(txHash *chainhash.Hash) (*StoredBurningTransaction, error) {
	var storedTx *StoredBurningTransaction
	txHashBytes := txHash.CloneBytes()

	err := is.db.View(func(tx kvdb.RTx) error {
		txBucket := tx.ReadBucket(burningTxBucketName)
		if txBucket == nil {
			return ErrCorruptedTransactionsDb
		}

		maybeTx := txBucket.Get(txHashBytes)
		if maybeTx == nil {
			return ErrTransactionNotFound
		}

		var storedTxProto proto.BurningTransaction
		if err := pm.Unmarshal(maybeTx, &storedTxProto); err != nil {
			return ErrCorruptedTransactionsDb
		}

		txFromDb, err := protoBurningTxToStoredBurningTx(&storedTxProto)
		if err != nil {
			return err
		}

		storedTx = txFromDb
		return nil
	}, func() {})

	if err != nil && !errors.Is(err, ErrTransactionNotFound) {
		return nil, err
	}

	return storedTx, nil
}

func protoBurningTxToStoredBurningTx(protoTx *proto.BurningTransaction) (*StoredBurningTransaction, error) {
	var burningTx wire.MsgTx
	err := burningTx.Deserialize(bytes.NewReader(protoTx.TransactionBytes))
	if err != nil {
		return nil, fmt.Errorf("invalid burning tx: %w", err)
	}

	vaultTxHash, err := chainhash.NewHash(protoTx.VaultTxHash)
	if err != nil {
		return nil, fmt.Errorf("invalid staking tx hash")
	}

	return &StoredBurningTransaction{
		Tx:          &burningTx,
		VaultTxHash: vaultTxHash,
	}, nil
}

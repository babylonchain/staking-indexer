package indexderdb

import "github.com/lightningnetwork/lnd/kvdb"

var (
	// mapping tx hash -> Transaction
	transactionBucketName = []byte("transactions")
)

type TransactionStore struct {
	db kvdb.Backend
}

// NewTransactionStore returns a new store backed by db
func NewTransactionStore(db kvdb.Backend) (*TransactionStore,
	error) {

	store := &TransactionStore{db}
	if err := store.initBuckets(); err != nil {
		return nil, err
	}

	return store, nil
}

func (c *TransactionStore) initBuckets() error {
	return kvdb.Batch(c.db, func(tx kvdb.RwTx) error {
		_, err := tx.CreateTopLevelBucket(transactionBucketName)
		if err != nil {
			return err
		}

		return nil
	})
}

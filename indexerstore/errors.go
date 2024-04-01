package indexerstore

import "errors"

var (
	// ErrCorruptedTransactionsDb For some reason, db on disk representation have changed
	ErrCorruptedTransactionsDb = errors.New("transactions db is corrupted")

	// ErrTransactionNotFound The transaction we try update is not found in db
	ErrTransactionNotFound = errors.New("transaction not found")

	// ErrDuplicateTransaction The transaction we try to add already exists in db
	ErrDuplicateTransaction = errors.New("transaction already exists")

	// ErrCorruptedStateDb For some reason, db on disk representation have changed
	ErrCorruptedStateDb = errors.New("state db is corrupted")

	// ErrLastProcessedHeightNotFound the last processed height is not found in db
	ErrLastProcessedHeightNotFound = errors.New("last processed height not found")
)

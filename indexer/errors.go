package indexer

import "errors"

var (
	// ErrInvalidGlobalParameters the global parameters are invalid
	ErrInvalidGlobalParameters = errors.New("invalid parameters")

	// ErrInvalidUnbondingTx the transaction spends the unbonding path but is invalid
	ErrInvalidUnbondingTx = errors.New("invalid unbonding tx")

	// ErrInvalidStakingTx the stake transaction is invalid as it does not follow the global parameters
	ErrInvalidStakingTx = errors.New("invalid staking tx")
)

type IndexerError struct {
	Err     error
	Message string
}

// NewIndexerError creates a new IndexerError
func NewIndexerError(err error, message string) *IndexerError {
	return &IndexerError{
		Err:     err,
		Message: message,
	}
}

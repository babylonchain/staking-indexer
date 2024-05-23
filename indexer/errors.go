package indexer

import "errors"

var (
	// ErrInvalidUnbondingTx the transaction spends the unbonding path but is invalid
	ErrInvalidUnbondingTx = errors.New("invalid unbonding tx")

	// ErrInvalidStakingTx the stake transaction is invalid as it does not follow the global parameters
	ErrInvalidStakingTx = errors.New("invalid staking tx")

	// ErrInvalidWithdrawalTx the withdrawal transaction is invalid as it does not unlock the expected time lock path
	ErrInvalidWithdrawalTx = errors.New("invalid withdrawal tx")
)

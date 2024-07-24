package indexer

import "errors"

var (
	ErrInvalidBurningTx = errors.New("invalid burning tx")

	ErrInvalidVaultTx = errors.New("invalid vault tx")
)

package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/pkg/errors"
)

// TxFn is a type for a function that will be called with an initialized
// transaction. It's used to make database-related code cleaner and safer by
// guaranteeing that opened transactions always either commit or rollback.
type TxFn func(*sql.Tx) error

// WithTransaction creates a new transaction and handles its rollback or
// commit.
//
// The transaction is rolled back if a non-nil error is returned. Otherwise, it
// commits.
func WithTransaction(db *sql.DB, fn TxFn) (resErr error) {
	tx, err := db.Begin()
	if err != nil {
		resErr = errors.Wrap(err, "Failed to start transaction")
		return
	}

	defer func() {
		if p := recover(); p != nil {
			rollback(tx)
			panic(p)
		} else if resErr != nil {
			rollback(tx)
		} else {
			resErr = tx.Commit()
		}
	}()

	resErr = fn(tx)
	return
}

//
// Private functions
//

func rollback(tx *sql.Tx) {
	err := tx.Rollback()
	if err != nil {
		// Log the error but don't return it. This error should never
		// supersede the original one returned by fn.
		fmt.Fprintf(os.Stderr, "Error on rollback: %v\n", err)
	}
}

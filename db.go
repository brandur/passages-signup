package main

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// TxFn is a type for a function that will be called with an initialized
// transaction. It's used to make database-related code cleaner and safer by
// guaranteeing that opened transactions always either commit or rollback.
type TxFn func(*sql.Tx) error

// Not initialized by default. Access through openDB instead.
var db *sql.DB
var dbMutex sync.RWMutex

// OpenDB lazily gets a database pointer. It ensures only one is created for
// the current program and is safe for concurrent access.
func OpenDB() (*sql.DB, error) {
	dbMutex.RLock()
	var localDB = db
	dbMutex.RUnlock()

	if localDB != nil {
		return db, nil
	}

	// No database connection yet, so initialize one.
	dbMutex.Lock()
	defer dbMutex.Unlock()

	// Multiple Goroutines may have passed through the read phase nearly
	// simultaneously (even if only one will have acquired the lock), so verify
	// that the connection still hasn't been set by a previous Goroutine that
	// beat us.
	if db != nil {
		return db, nil
	}

	var err error
	db, err = sql.Open("postgres", conf.DatabaseURL)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxLifetime(60 * time.Second)
	db.SetMaxOpenConns(conf.DatabaseMaxOpenConns)

	return db, nil
}

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

package testhelpers

import (
	"database/sql"
	"log"
	"testing"

	_ "github.com/lib/pq"
	assert "github.com/stretchr/testify/require"
)

const (
	DatabaseURL = "postgres://localhost/passages-signup-test?sslmode=disable"
	TestEmail   = "foo@example.com"
)

// ConnectDB opens a connection to the test database.
func ConnectDB(t *testing.T) *sql.DB {
	log.Printf("Connecting to test database")
	db, err := sql.Open("postgres", DatabaseURL)
	assert.NoError(t, err)
	return db
}

// ResetDB resets the database between test runs.
//
// Note that we don't do anything sophisticated, so if the application is made
// any more complex (new tables added, etc.), this will likely need to be
// updated.
func ResetDB(t *testing.T, db *sql.DB) {
	_, err := db.Exec(`TRUNCATE TABLE signup`)
	assert.NoError(t, err)
}

// WithTestTransaction is similar to WithTransaction except that it always
// rolls back the transaction. This is useful in test environments where we
// want to discard all results within a single test case.
func WithTestTransaction(t *testing.T, db *sql.DB, fn func(*sql.Tx)) {
	log.Printf("Starting test transaction")
	tx, err := db.Begin()
	assert.NoError(t, err)

	log.Printf("Running test body in test transaction")
	fn(tx)

	doRollback := func() {
		err = tx.Rollback()
		if err != nil {
			t.Logf("Error rolling back test transaction: %v", err)
		}
	}
	doRollback()

	defer func() {
		if p := recover(); p != nil {
			doRollback()
			panic(p)
		}
	}()
}

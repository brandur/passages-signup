package testhelpers

import (
	"database/sql"
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

package testhelpers

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	_ "github.com/lib/pq" // blank import recommended by pq
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/brandur/passages-signup/db"
)

const (
	TestEmail     = "foo@example.com"
	TestPublicURL = "https://passages.example.com"

	testDatabaseURL = "postgres://localhost/passages-signup-test?sslmode=disable"
)

var dbPool *pgxpool.Pool

func init() {
	var err error
	dbPool, err = db.Connect(context.Background(), &db.ConnectConfig{
		ApplicationName: "passages-signup-tests",
		DatabaseURL:     testDatabaseURL,
	})
	if err != nil {
		logrus.Fatalf("Error connecting to test database: %v", err)
	}
}

// TestTx returns a test transaction that's automatically rolled back on test
// cleanup. Targets the main database.
func TestTx(ctx context.Context, tb testing.TB) pgx.Tx { //nolint:ireturn
	tb.Helper()

	tx, err := dbPool.Begin(ctx)
	require.NoError(tb, err)

	tb.Cleanup(func() {
		// Tests inherit context from `t.Context()` which is cancelled after
		// tests run and before calling clean up. We need a non-cancelled
		// context to issue rollback here, so use a bit of a bludgeon to do so
		// with `context.WithoutCancel()`.
		ctx := context.WithoutCancel(ctx)

		err := tx.Rollback(ctx)
		require.NoError(tb, err)
	})

	return tx
}

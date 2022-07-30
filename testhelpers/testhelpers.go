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

// WithTestTransaction is similar to WithTransaction except that it always
// rolls back the transaction. This is useful in test environments where we
// want to discard all results within a single test case.
func WithTestTransaction(ctx context.Context, t *testing.T, f func(ctx context.Context, tx pgx.Tx)) {
	t.Helper()

	logrus.Infof("Starting test transaction")
	tx, err := dbPool.Begin(ctx)
	require.NoError(t, err)

	defer func() {
		err := tx.Rollback(ctx)
		require.NoError(t, err)
	}()

	logrus.Infof("Running test body in test transaction")
	f(ctx, tx)
}

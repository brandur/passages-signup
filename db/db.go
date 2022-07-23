package db

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"
)

var validate = validator.New()

// ConnectConfig contains configuration option to create a Postgres connection
// pool. We mandate some configuration that's not normally required (e.g.
// `application_name`) for operational reasons.
type ConnectConfig struct {
	// ApplicationName is the name under which queries issued by the connections
	// are registered with Postgres. This is useful for operators looking at
	// ongoing operations in the database so that they can easily determine the
	// origin of a problematic query.
	ApplicationName string `validate:"required"`

	// DatabaseURL is a typical connection string of the form `postgres://`.
	DatabaseURL string `validate:"required"`
}

func Connect(ctx context.Context, config *ConnectConfig) (*pgxpool.Pool, error) {
	if err := validate.Struct(config); err != nil {
		return nil, xerrors.Errorf("invalid database config: %w", err)
	}

	// Acquire the connection parameters from the standard set of PostgreSQL
	// connection parameters
	pgxConfig, err := pgxpool.ParseConfig(config.DatabaseURL)
	if err != nil {
		return nil, xerrors.Errorf("error parsing config: %w", err)
	}

	pgxConfig.MaxConns = 20
	pgxConfig.ConnConfig.RuntimeParams["application_name"] = config.ApplicationName

	// Idle in transaction should always be longer than statement timeout
	// because a statement executing also increments in the idle in transaction
	// timer.
	pgxConfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = strconv.Itoa(int((15 * time.Second).Milliseconds()))
	pgxConfig.ConnConfig.RuntimeParams["statement_timeout"] = strconv.Itoa(int((10 * time.Second).Milliseconds()))

	// Load the connection configuration into the connection pool and open the
	// pool
	pool, err := pgxpool.ConnectConfig(ctx, pgxConfig)
	if err != nil {
		return nil, xerrors.Errorf("error connecting to Postgres: %w", err)
	}

	return pool, nil
}

// TXStarter allows a transaction to be started on either a pool or another
// transaction.
type TXStarter interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// WithTransaction creates a new transaction and handles its rollback or commits.
// The transaction is rolled back if a non-nil error is returned. Otherwise, it
// commits.
func WithTransaction(ctx context.Context, starter TXStarter, f func(ctx context.Context, tx pgx.Tx) error) error {
	tx, err := starter.Begin(ctx)
	if err != nil {
		return xerrors.Errorf("error starting transaction: %w", err)
	}

	defer func() {
		// It's safe to call Rollback even if the transaction committed
		// successfully.
		if err := tx.Rollback(ctx); err != nil {
			if !errors.Is(err, pgx.ErrTxClosed) {
				logrus.Errorf("Error rolling back: %v", err)
			}
		}
	}()

	if err := f(ctx, tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return xerrors.Errorf("error committing transaction: %w", err)
	}

	return nil
}

package command

import (
	"context"

	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/brandur/passages-signup/mailclient"
)

// SignupFinisher takes an email that's already started the signup process and
// fully adds it to the mailing list. It does this based on Token, which is
// received through a secret URL.
type SignupFinisher struct {
	ListAddress string         `validate:"required"`
	MailAPI     mailclient.API `validate:"required"`
	Token       string         `validate:"required"`
}

// Run executes the mediator.
func (c *SignupFinisher) Run(ctx context.Context, tx pgx.Tx) (*SignupFinisherResult, error) {
	logrus.Infof("SignupFinisher running")

	if err := validate.Struct(c); err != nil {
		return nil, xerrors.Errorf("error validating command: %w", err)
	}

	var id *int64
	var email *string
	err := tx.QueryRow(ctx, `
		SELECT id, email
		FROM signup
		WHERE token = $1
	`, c.Token).Scan(&id, &email)

	// No such token.
	if errors.Is(err, pgx.ErrNoRows) {
		return &SignupFinisherResult{TokenNotFound: true}, nil
	}

	// Handle all other database-related errors.
	if err != nil {
		return nil, xerrors.Errorf("error querying for token: %w", err)
	}

	// Make sure to update the row to indicate that we've successfully
	// completed the signup. Note that this run is fully idempotent. If the
	// next API call fails, the user can safely retry this as many as many
	// times as necessary.
	_, err = tx.Exec(ctx, `
		UPDATE signup
		SET completed_at = NOW()
		WHERE id = $1
	`, *id)
	if err != nil {
		return nil, xerrors.Errorf("error updating record: %w", err)
	}

	logrus.Infof("Adding %v to the list\n", *email)
	err = c.MailAPI.AddMember(ctx, c.ListAddress, *email)
	if err != nil {
		return nil, xerrors.Errorf("error adding email to list: %w", err)
	}

	return &SignupFinisherResult{Email: *email, SignupFinished: true}, nil
}

// SignupFinisherResult holds the results of a successful run of
// SignupFinisher.
type SignupFinisherResult struct {
	Email          string
	SignupFinished bool
	TokenNotFound  bool
}

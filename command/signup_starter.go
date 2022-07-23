package command

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/aymerick/douceur/inliner"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/brandur/passages-signup/mailclient"
	"github.com/brandur/passages-signup/ptemplate"
)

const (
	// Maximum of number of times we'll ever try to send a confirmation email
	// to a particular email address.
	maxNumSignupAttempts = 3

	// If we've already tried to confirm a signup by sending a confirmation
	// email, we won't try to send another confirmation email for at least this
	// many hours, even if a user submits the forma again.
	noResendHours = 24
)

// ErrInvalidEmail is the error that's returned if a given email address
// didn't match a regex to check for email validity.
var ErrInvalidEmail = errors.New("That doesn't look like a valid email address")

var emailRegexp = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

// SignupStarter takes an email and begins the signup process or it.
//
// Usually that involves dispatching an email to the address that contains a
// secret URL that can be used to fully confirm the signup. If the email is
// already signed up, then the command is a no-op. If the confirmation email
// was dispatched but not yet confirmed, it may be resent, but only if outside
// a rate limited window.
type SignupStarter struct {
	Email          string              `validate:"required"`
	ListAddress    string              `validate:"required"`
	MailAPI        mailclient.API      `validate:"required"`
	Renderer       *ptemplate.Renderer `validate:"required"`
	ReplyToAddress string              `validate:"required"`
}

// Run executes the mediator.
func (c *SignupStarter) Run(ctx context.Context, tx pgx.Tx) (*SignupStarterResult, error) {
	logrus.Infof("SignupStarter running")

	if err := validate.Struct(c); err != nil {
		return nil, xerrors.Errorf("error validating command: %w", err)
	}

	// We know that a simple regexp validation won't detect all invalid email
	// addresses, so to some extent we'll be relying on Mailgun to do some of
	// that work for us.
	if !emailRegexp.MatchString(c.Email) {
		return nil, ErrInvalidEmail
	}

	var id *int64
	var completedAt *time.Time
	var lastSentAt *time.Time
	var numAttempts *int64
	var token *string
	err := tx.QueryRow(ctx, `
		SELECT id, completed_at, last_sent_at, num_attempts, token
		FROM signup
		WHERE email = $1
	`, c.Email).Scan(&id, &completedAt, &lastSentAt, &numAttempts, &token)

	// The happy path: if we have nothing in the database, then just run the
	// process from scratch.
	if errors.Is(err, pgx.ErrNoRows) {
		token := uuid.New().String()

		_, err = tx.Exec(ctx, `
			INSERT INTO signup
				(email, token)
			VALUES
				($1, $2)
		`, c.Email, token)
		if err != nil {
			return nil, xerrors.Errorf("error inserting singup row: %w", err)
		}

		err = c.sendConfirmationMessage(ctx, token)
		if err != nil {
			return nil, xerrors.Errorf("error sending confirmation message: %w", err)
		}

		return &SignupStarterResult{NewSignup: true}, nil
	}

	// Handle all other database-related errors.
	if err != nil {
		return nil, xerrors.Errorf("error querying for existing record: %w", err)
	}

	if completedAt == nil && *numAttempts >= maxNumSignupAttempts {
		logrus.Infof("Too many signup attempts for email: %s", c.Email)
		return &SignupStarterResult{MaxNumAttempts: true}, nil
	}

	// Note that we don't bail early even if the record appears to be completed
	// because if the user was previously subscribed but then unsubscribed, we
	// won't know about the unsubscription because it happens entirely through
	// Mailgun.
	//
	// The side effect is that we may send a signup confirmation to a user who
	// is already subscribed, but that's not a big deal.

	// If we sent the last confirmation email recently, then don't send it
	// again. This gives a malicious actor less opportunity to spam an innocent
	// recipient.
	//
	// We do want to eventually sent another email in case the user signed up
	// before but failed to complete the process, and now wants to try again.
	// The duration parameter may need to be tweaked.
	if lastSentAt.After(time.Now().Add(-noResendHours * time.Hour)) {
		logrus.Infof("Last send was too soon so not re-sending confirmation, %s",
			c.Email)
		return &SignupStarterResult{ConfirmationRateLimited: true}, nil
	}

	// Update the number of attempts, but only if this user hasn't already
	// completed the signup flow.
	if completedAt == nil {
		*numAttempts++
	}

	// Otherwise, update the timestamp and number of attempts. Re-send the
	// confirmation message.
	_, err = tx.Exec(ctx, `
		UPDATE signup
		SET
		  last_sent_at = NOW(),
		  num_attempts = $1
		WHERE id = $2
	`, *numAttempts, *id)
	if err != nil {
		return nil, xerrors.Errorf("error updating existing record: %w", err)
	}

	// Re-send confirmation.
	err = c.sendConfirmationMessage(ctx, *token)
	if err != nil {
		return nil, xerrors.Errorf("error sending confirmation email: %w", err)
	}

	return &SignupStarterResult{ConfirmationResent: true}, nil
}

func (c *SignupStarter) sendConfirmationMessage(ctx context.Context, token string) error {
	logrus.Infof("Sending confirmation mail to %v with token %v\n", c.Email, token)

	subject := c.Renderer.NewsletterMeta.Name + " signup confirmation"

	buf := new(bytes.Buffer)
	err := c.Renderer.RenderTemplate(buf, "views/messages/confirm_plain", map[string]interface{}{
		"token": token,
	})
	if err != nil {
		return xerrors.Errorf("error rendering confirmation email (plain): %w", err)
	}
	confirmPlain := strings.TrimSpace(buf.String())

	buf = new(bytes.Buffer)
	err = c.Renderer.RenderTemplate(buf, "views/messages/confirm", map[string]interface{}{
		"token": token,
	})
	if err != nil {
		return xerrors.Errorf("error rendering confirmation email (HTML): %w", err)
	}
	confirmHTML := buf.String()

	// Inline CSS styling (because that's the only way mail clients will
	// support it).
	confirmHTML, err = inliner.Inline(confirmHTML)
	if err != nil {
		return xerrors.Errorf("error inlining CSS styling: %w", err)
	}

	return c.MailAPI.SendMessage(ctx, &mailclient.SendMessageParams{ //nolint:wrapcheck
		ContentsHTML:   confirmHTML,
		ContentsPlain:  confirmPlain,
		ListAddress:    c.ListAddress,
		NewsletterName: c.Renderer.NewsletterMeta.Name,
		Recipient:      c.Email,
		ReplyTo:        c.ReplyToAddress,
		Subject:        subject,
	})
}

// SignupStarterResult holds the results of a successful run of SignupStarter.
type SignupStarterResult struct {
	ConfirmationRateLimited bool
	ConfirmationResent      bool
	MaxNumAttempts          bool
	NewSignup               bool
}

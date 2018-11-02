package main

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/satori/go.uuid"
)

var (
	ErrInvalidEmail = errors.New("That doesn't look like a valid email address")
)

var (
	emailRegexp = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
)

//
// SignupFinisher
//

// SignupFinisher takes an email that's already started the signup process and
// fully adds it to the mailing list. It does this based on Token, which is
// received through a secret URL.
type SignupFinisher struct {
	MailAPI MailAPI
	Token   string
}

// Run executes the mediator.
func (c *SignupFinisher) Run(tx *sql.Tx) (*SignupFinisherResult, error) {
	var id *int64
	var email *string
	err := tx.QueryRow(`
		SELECT id, email
		FROM signup
		WHERE token = $1
	`, c.Token).Scan(&id, &email)

	// No such token.
	if err == sql.ErrNoRows {
		return &SignupFinisherResult{TokenNotFound: true}, nil
	}

	// Handle all other database-related errors.
	if err != nil {
		return nil, errors.Wrap(err, "Failed to query for token")
	}

	// Make sure to update the row to indicate that we've successfully
	// completed the signup. Note that this run is fully idempotent. If the
	// next API call fails, the user can safely retry this as many as many
	// times as necessary.
	_, err = tx.Exec(`
		UPDATE signup
		SET completed_at = NOW(),
			last_sent_at = NULL
		WHERE id = $1
	`, *id)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to update existing record")
	}

	log.Printf("Adding %v to the list\n", *email)
	err = c.MailAPI.AddMember(mailList, *email)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to add email to list")
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

//
// SignupStarter
//

// SignupStarter takes an email and begins the signup process or it.
//
// Usually that involves dispatching an email to the address that contains a
// secret URL that can be used to fully confirm the signup. If the email is
// already signed up, then the command is a no-op. If the confirmation email
// was dispatched but not yet confirmed, it may be resent, but only if outside
// a rate limited window.
type SignupStarter struct {
	Email   string
	MailAPI MailAPI
}

// Run executes the mediator.
func (c *SignupStarter) Run(tx *sql.Tx) (*SignupStarterResult, error) {
	// We know that a simple regexp validation won't detect all invalid email
	// addresses, so to some extent we'll be relying on Mailgun to do some of
	// that work for us.
	if !emailRegexp.MatchString(c.Email) {
		return nil, ErrInvalidEmail
	}

	var id *int64
	var lastSentAt, completedAt *time.Time
	var token *string
	err := tx.QueryRow(`
		SELECT id, completed_at, last_sent_at, token
		FROM signup
		WHERE email = $1
	`, c.Email).Scan(&id, &completedAt, &lastSentAt, &token)

	// The happy path: if we have nothing in the database, then just run the
	// process from scratch.
	if err == sql.ErrNoRows {
		tokenStruct, err := uuid.NewV4()
		if err != nil {
			return nil, errors.Wrap(err, "Failed to generate UUID")
		}

		token := tokenStruct.String()

		_, err = tx.Exec(`
			INSERT INTO signup
				(email, token)
			VALUES
				($1, $2)
		`, c.Email, token)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to insert new signup row")
		}

		err = c.sendConfirmationMessage(token)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to send confirmation message")
		}

		return &SignupStarterResult{NewSignup: true}, nil
	}

	// Handle all other database-related errors.
	if err != nil {
		return nil, errors.Wrap(err, "Failed to query for existing record")
	}

	// If the signup process is already fully completed, we're done.
	if completedAt != nil {
		return &SignupStarterResult{AlreadySubscribed: true}, nil
	}

	// If we sent the last confirmation email recently, then don't send it
	// again. This gives a malicious actor less opportunity to spam an innocent
	// recipient.
	//
	// We do want to eventually sent another email in case the user signed up
	// before but failed to complete the process, and now wants to try again.
	// The duration parameter may need to be tweaked.
	if (*lastSentAt).After(time.Now().Add(-24 * time.Hour)) {
		log.Printf("Last send was too soon so not re-sending confirmation")
		return &SignupStarterResult{ConfirmationRateLimited: true}, nil
	}

	// Otherwise, update the timestamp and re-send the confirmation message.
	_, err = tx.Exec(`
		UPDATE signup
		SET last_sent_at = NOW()
		WHERE id = $1
	`, *id)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to update existing record")
	}

	// Re-send confirmation.
	err = c.sendConfirmationMessage(*token)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to send confirmation email")
	}

	return &SignupStarterResult{ConfirmationResent: true}, nil
}

func (c *SignupStarter) sendConfirmationMessage(token string) error {
	log.Printf("Sending confirmation mail to %v with token %v\n", c.Email, token)

	subject := "Passages & Glass signup confirmation"
	contents := strings.TrimSpace(fmt.Sprintf(`
Hello! I recently received a request to join the Passages & Glass mailing list. See here for more information:
	https://brandur.org/newsletter

If you'd still like to join, please confirm by clicking this link:
	%s/confirm/%s

If you received this email in error, it's safe to ignore it. You will stay unsubscribed by default.
	`, conf.PublicURL, token))

	return c.MailAPI.SendMessage(c.Email, subject, contents)
}

// SignupStarterResult holds the results of a successful run of SignupStarter.
type SignupStarterResult struct {
	AlreadySubscribed       bool
	ConfirmationRateLimited bool
	ConfirmationResent      bool
	NewSignup               bool
}

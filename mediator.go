package main

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/pkg/errors"
	"github.com/satori/go.uuid"
)

const (
	mailDomain = "list.brandur.org"
	mailList   = "passages@" + mailDomain
)

var (
	ErrInvalidEmail = errors.New("That doesn't look like a valid email address")

	emailRegexp = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
)

//
// SignupFinisher
//

// SignupFinisher takes an email that's already started the signup process and
// fully adds it to the mailing list. It does this based on Token, which is
// received through a secret URL.
type SignupFinisher struct {
	DB      *sql.DB
	MailAPI MailAPI
	Token   string
}

// Run executes the mediator.
func (c *SignupFinisher) Run() (res *SignupFinisherResult, resErr error) {
	tx, err := c.DB.Begin()
	if err != nil {
		resErr = errors.Wrap(err, "Failed to start transaction")
		return
	}

	defer func() {
		if resErr == nil {
			resErr = tx.Commit()
		} else {
			err := tx.Rollback()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Rollback error = %+v\n", err)
			}
		}
	}()

	var id *int64
	var email *string
	err = tx.QueryRow(`
		SELECT id, email
		FROM signup
		WHERE token = $1
	`, c.Token).Scan(&id, &email)

	// No such token.
	if err == sql.ErrNoRows {
		res = &SignupFinisherResult{TokenNotFound: true}
		return
	}

	// Handle all other database-related errors.
	if err != nil {
		resErr = errors.Wrap(err, "Failed to query for token")
		return
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
		resErr = errors.Wrap(err, "Failed to update existing record")
		return
	}

	fmt.Printf("Adding %v to the list\n", *email)
	err = c.MailAPI.AddMember(mailList, *email)
	if err != nil {
		resErr = errors.Wrap(err, "Failed to add email to list")
		return
	}

	res = &SignupFinisherResult{Email: *email, SignupFinished: true}
	return
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
	DB      *sql.DB
	Email   string
	MailAPI MailAPI
}

// Run executes the mediator.
func (c *SignupStarter) Run() (res *SignupStarterResult, resErr error) {
	tx, err := c.DB.Begin()
	if err != nil {
		resErr = errors.Wrap(err, "Failed to start transaction")
		return
	}

	defer func() {
		if resErr == nil {
			resErr = tx.Commit()
		} else {
			err := tx.Rollback()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Rollback error = %+v\n", err)
			}
		}
	}()

	// We know that a simple regexp validation won't detect all invalid email
	// addresses, so to some extent we'll be relying on Mailgun to do some of
	// that work for us.
	if !emailRegexp.MatchString(c.Email) {
		resErr = ErrInvalidEmail
		return
	}

	var id *int64
	var lastSentAt, completedAt *time.Time
	var token *string
	err = tx.QueryRow(`
		SELECT id, completed_at, last_sent_at, token
		FROM signup
		WHERE email = $1
	`, c.Email).Scan(&id, &completedAt, &lastSentAt, &token)

	// The happy path: if we have nothing in the database, then just run the
	// process from scratch.
	if err == sql.ErrNoRows {
		tokenStruct, err := uuid.NewV4()
		if err != nil {
			resErr = errors.Wrap(err, "Failed to generate UUID")
			return
		}

		token := tokenStruct.String()

		_, err = tx.Exec(`
			INSERT INTO signup
				(email, token)
			VALUES
				($1, $2)
		`, c.Email, token)
		if err != nil {
			resErr = errors.Wrap(err, "Failed to insert new signup row")
			return
		}

		err = c.sendConfirmationMessage(token)
		if err != nil {
			resErr = errors.Wrap(err, "Failed to send confirmation message")
			return
		}

		res = &SignupStarterResult{NewSignup: true}
		return
	}

	// Handle all other database-related errors.
	if err != nil {
		resErr = errors.Wrap(err, "Failed to query for existing record")
		return
	}

	// If the signup process is already fully completed, we're done.
	if completedAt != nil {
		res = &SignupStarterResult{AlreadySubscribed: true}
		return
	}

	// If we sent the last confirmation email recently, then don't send it
	// again. This gives a malicious actor less opportunity to spam an innocent
	// recipient.
	//
	// We do want to eventually sent another email in case the user signed up
	// before but failed to complete the process, and now wants to try again.
	// The duration parameter may need to be tweaked.
	if (*lastSentAt).After(time.Now().Add(-24 * time.Hour)) {
		fmt.Printf("Last send was too soon so not re-sending confirmation")
		res = &SignupStarterResult{ConfirmationRateLimited: true}
		return
	}

	// Otherwise, update the timestamp and re-send the confirmation message.
	_, err = tx.Exec(`
		UPDATE signup
		SET last_sent_at = NOW()
		WHERE id = $1
	`, *id)
	if err != nil {
		resErr = errors.Wrap(err, "Failed to update existing record")
		return
	}

	// Re-send confirmation.
	err = c.sendConfirmationMessage(*token)
	if err != nil {
		resErr = errors.Wrap(err, "Failed to send confirmation email")
		return
	}

	res = &SignupStarterResult{ConfirmationResent: true}
	return
}

func (c *SignupStarter) sendConfirmationMessage(token string) error {
	fmt.Printf("Sending confirmation mail to %v with token %v\n", c.Email, token)
	return c.MailAPI.SendMessage(c.Email, "hello")
}

// SignupStarterResult holds the results of a successful run of SignupStarter.
type SignupStarterResult struct {
	AlreadySubscribed       bool
	ConfirmationRateLimited bool
	ConfirmationResent      bool
	NewSignup               bool
}

package main

import (
	"database/sql"
	"testing"

	"github.com/brandur/passages-signup/testhelpers"
	assert "github.com/stretchr/testify/require"
)

func TestSignupFinisher(t *testing.T) {
	db := testhelpers.ConnectDB(t)

	// Normal signup finish
	t.Run("FinishSignup", func(t *testing.T) {
		testhelpers.WithTestTransaction(t, db, func(tx *sql.Tx) {
			token := "test-token"

			// Manually insert a record ready to be finished
			_, err := tx.Exec(`
				INSERT INTO signup
					(email, token)
				VALUES
					($1, $2)
			`, testhelpers.TestEmail, token)
			assert.NoError(t, err)

			mailAPI := NewFakeMailAPI()
			mediator := signupFinisher(mailAPI, token)

			res, err := mediator.Run(tx)
			assert.NoError(t, err)

			assert.Equal(t, testhelpers.TestEmail, res.Email)
			assert.True(t, res.SignupFinished)
			assert.False(t, res.TokenNotFound)

			assert.Equal(t, 1, len(mailAPI.MembersAdded))
			assert.Equal(t, testhelpers.TestEmail, mailAPI.MembersAdded[0].Email)

			//
			// Make sure it's idempotent by running it immediately again with the
			// same inputs
			//

			res, err = mediator.Run(tx)
			assert.NoError(t, err)

			assert.Equal(t, testhelpers.TestEmail, res.Email)
			assert.True(t, res.SignupFinished)
			assert.False(t, res.TokenNotFound)

			assert.Equal(t, 2, len(mailAPI.MembersAdded))
			assert.Equal(t, testhelpers.TestEmail, mailAPI.MembersAdded[1].Email)
		})
	})

	// Unknown token
	t.Run("UnknownToken", func(t *testing.T) {
		testhelpers.WithTestTransaction(t, db, func(tx *sql.Tx) {
			mailAPI := NewFakeMailAPI()
			mediator := signupFinisher(mailAPI, "not-a-token")

			res, err := mediator.Run(tx)
			assert.NoError(t, err)

			assert.Empty(t, res.Email)
			assert.False(t, res.SignupFinished)
			assert.True(t, res.TokenNotFound)

			assert.Equal(t, 0, len(mailAPI.MembersAdded))
		})
	})
}

func TestSignupStarter(t *testing.T) {
	db := testhelpers.ConnectDB(t)

	// New signup
	t.Run("NewSignup", func(t *testing.T) {
		testhelpers.WithTestTransaction(t, db, func(tx *sql.Tx) {

			mailAPI := NewFakeMailAPI()
			mediator := signupStarter(mailAPI, testhelpers.TestEmail)

			res, err := mediator.Run(tx)
			assert.NoError(t, err)

			assert.False(t, res.AlreadySubscribed)
			assert.False(t, res.ConfirmationRateLimited)
			assert.False(t, res.ConfirmationResent)
			assert.True(t, res.NewSignup)

			assert.Equal(t, 1, len(mailAPI.MessagesSent))
			assert.Equal(t, testhelpers.TestEmail, mailAPI.MessagesSent[0].Email)
		})
	})

	// Email that's already subscribed
	t.Run("AlreadySubscribed", func(t *testing.T) {
		testhelpers.WithTestTransaction(t, db, func(tx *sql.Tx) {

			// Manually insert a finished record
			_, err := tx.Exec(`
			INSERT INTO signup
				(email, token, completed_at)
			VALUES
				($1, 'not-a-real-token', NOW())
		`, testhelpers.TestEmail)
			assert.NoError(t, err)

			mailAPI := NewFakeMailAPI()
			mediator := signupStarter(mailAPI, testhelpers.TestEmail)

			res, err := mediator.Run(tx)
			assert.NoError(t, err)

			assert.True(t, res.AlreadySubscribed)
			assert.False(t, res.ConfirmationRateLimited)
			assert.False(t, res.ConfirmationResent)
			assert.False(t, res.NewSignup)

			assert.Equal(t, 0, len(mailAPI.MessagesSent))
		})
	})

	// Email already in progress, but with signup not completed
	t.Run("ConfirmationResent", func(t *testing.T) {
		testhelpers.WithTestTransaction(t, db, func(tx *sql.Tx) {

			// Manually insert a finished record
			_, err := tx.Exec(`
			INSERT INTO signup
				(email, token, last_sent_at)
			VALUES
				($1, 'not-a-real-token', NOW() - '1 month'::interval)
		`, testhelpers.TestEmail)
			assert.NoError(t, err)

			mailAPI := NewFakeMailAPI()
			mediator := signupStarter(mailAPI, testhelpers.TestEmail)

			res, err := mediator.Run(tx)
			assert.NoError(t, err)

			assert.False(t, res.AlreadySubscribed)
			assert.False(t, res.ConfirmationRateLimited)
			assert.True(t, res.ConfirmationResent)
			assert.False(t, res.NewSignup)

			assert.Equal(t, 1, len(mailAPI.MessagesSent))
			assert.Equal(t, testhelpers.TestEmail, mailAPI.MessagesSent[0].Email)
		})
	})

	// Email already in progress, but too soon after last attempt
	t.Run("ConfirmationRateLimited", func(t *testing.T) {
		testhelpers.WithTestTransaction(t, db, func(tx *sql.Tx) {
			// Manually insert a finished record
			_, err := tx.Exec(`
			INSERT INTO signup
				(email, token, last_sent_at)
			VALUES
				($1, 'not-a-real-token', NOW() - '1 hour'::interval)
		`, testhelpers.TestEmail)
			assert.NoError(t, err)

			mailAPI := NewFakeMailAPI()
			mediator := signupStarter(mailAPI, testhelpers.TestEmail)

			res, err := mediator.Run(tx)
			assert.NoError(t, err)

			assert.False(t, res.AlreadySubscribed)
			assert.True(t, res.ConfirmationRateLimited)
			assert.False(t, res.ConfirmationResent)
			assert.False(t, res.NewSignup)

			assert.Equal(t, 0, len(mailAPI.MessagesSent))
		})
	})

	// Invalid email address
	t.Run("InvalidEmail", func(t *testing.T) {
		testhelpers.WithTestTransaction(t, db, func(tx *sql.Tx) {
			mailAPI := NewFakeMailAPI()
			mediator := signupStarter(mailAPI, "blah-not-an-email")

			_, err := mediator.Run(tx)
			assert.Error(t, ErrInvalidEmail, err)
		})
	})
}

//
// Private functions
//

func signupFinisher(mailAPI MailAPI, token string) *SignupFinisher {
	return &SignupFinisher{
		MailAPI: mailAPI,
		Token:   token,
	}
}

func signupStarter(mailAPI MailAPI, email string) *SignupStarter {
	return &SignupStarter{
		Email:   email,
		MailAPI: mailAPI,
	}
}

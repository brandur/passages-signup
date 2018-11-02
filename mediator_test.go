package main

import (
	"database/sql"
	"testing"

	_ "github.com/lib/pq"
	assert "github.com/stretchr/testify/require"
)

func TestSignupFinisher(t *testing.T) {
	db := connectDB(t)

	// Normal signup finish
	t.Run("FinishSignup", func(t *testing.T) {
		resetDB(t, db)

		token := "test-token"

		// Manually insert a record ready to be finished
		_, err := db.Exec(`
			INSERT INTO signup
				(email, token)
			VALUES
				($1, $2)
		`, testEmail, token)
		assert.NoError(t, err)

		mailAPI := NewFakeMailAPI()
		mediator := signupFinisher(db, mailAPI, token)

		res, err := mediator.Run()
		assert.NoError(t, err)

		assert.True(t, res.SignupFinished)
		assert.False(t, res.TokenNotFound)

		assert.Equal(t, 1, len(mailAPI.MembersAdded))
		assert.Equal(t, testEmail, mailAPI.MembersAdded[0].Email)

		//
		// Make sure it's idempotent by running it immediately again with the
		// same inputs
		//

		res, err = mediator.Run()
		assert.NoError(t, err)

		assert.True(t, res.SignupFinished)
		assert.False(t, res.TokenNotFound)

		assert.Equal(t, 2, len(mailAPI.MembersAdded))
		assert.Equal(t, testEmail, mailAPI.MembersAdded[1].Email)
	})

	// Unknown token
	t.Run("UnknownToken", func(t *testing.T) {
		resetDB(t, db)
		mailAPI := NewFakeMailAPI()
		mediator := signupFinisher(db, mailAPI, "not-a-token")

		res, err := mediator.Run()
		assert.NoError(t, err)

		assert.False(t, res.SignupFinished)
		assert.True(t, res.TokenNotFound)

		assert.Equal(t, 0, len(mailAPI.MembersAdded))
	})
}

func TestSignupStarter(t *testing.T) {
	db := connectDB(t)

	// New signup
	t.Run("NewSignup", func(t *testing.T) {
		resetDB(t, db)

		mailAPI := NewFakeMailAPI()
		mediator := signupStarter(db, mailAPI, testEmail)

		res, err := mediator.Run()
		assert.NoError(t, err)

		assert.False(t, res.AlreadySubscribed)
		assert.False(t, res.ConfirmationRateLimited)
		assert.False(t, res.ConfirmationResent)
		assert.True(t, res.NewSignup)

		assert.Equal(t, 1, len(mailAPI.MessagesSent))
		assert.Equal(t, testEmail, mailAPI.MessagesSent[0].Email)
	})

	// Email that's already subscribed
	t.Run("AlreadySubscribed", func(t *testing.T) {
		resetDB(t, db)

		// Manually insert a finished record
		_, err := db.Exec(`
			INSERT INTO signup
				(email, token, completed_at)
			VALUES
				($1, 'not-a-real-token', NOW())
		`, testEmail)
		assert.NoError(t, err)

		mailAPI := NewFakeMailAPI()
		mediator := signupStarter(db, mailAPI, testEmail)

		res, err := mediator.Run()
		assert.NoError(t, err)

		assert.True(t, res.AlreadySubscribed)
		assert.False(t, res.ConfirmationRateLimited)
		assert.False(t, res.ConfirmationResent)
		assert.False(t, res.NewSignup)

		assert.Equal(t, 0, len(mailAPI.MessagesSent))
	})

	// Email already in progress, but with signup not completed
	t.Run("ConfirmationResent", func(t *testing.T) {
		resetDB(t, db)

		// Manually insert a finished record
		_, err := db.Exec(`
			INSERT INTO signup
				(email, token, last_sent_at)
			VALUES
				($1, 'not-a-real-token', NOW() - '1 month'::interval)
		`, testEmail)
		assert.NoError(t, err)

		mailAPI := NewFakeMailAPI()
		mediator := signupStarter(db, mailAPI, testEmail)

		res, err := mediator.Run()
		assert.NoError(t, err)

		assert.False(t, res.AlreadySubscribed)
		assert.False(t, res.ConfirmationRateLimited)
		assert.True(t, res.ConfirmationResent)
		assert.False(t, res.NewSignup)

		assert.Equal(t, 1, len(mailAPI.MessagesSent))
		assert.Equal(t, testEmail, mailAPI.MessagesSent[0].Email)
	})

	// Email already in progress, but too soon after last attempt
	t.Run("ConfirmationRateLimited", func(t *testing.T) {
		resetDB(t, db)

		// Manually insert a finished record
		_, err := db.Exec(`
			INSERT INTO signup
				(email, token, last_sent_at)
			VALUES
				($1, 'not-a-real-token', NOW() - '1 hour'::interval)
		`, testEmail)
		assert.NoError(t, err)

		mailAPI := NewFakeMailAPI()
		mediator := signupStarter(db, mailAPI, testEmail)

		res, err := mediator.Run()
		assert.NoError(t, err)

		assert.False(t, res.AlreadySubscribed)
		assert.True(t, res.ConfirmationRateLimited)
		assert.False(t, res.ConfirmationResent)
		assert.False(t, res.NewSignup)

		assert.Equal(t, 0, len(mailAPI.MessagesSent))
	})
}

//
// Private constants
//

const (
	databaseURL = "postgres://localhost/passages-signup-test?sslmode=disable"
	testEmail   = "foo@example.com"
)

//
// Private functions
//

func connectDB(t *testing.T) *sql.DB {
	db, err := sql.Open("postgres", databaseURL)
	assert.NoError(t, err)
	return db
}

func resetDB(t *testing.T, db *sql.DB) {
	_, err := db.Exec(`TRUNCATE TABLE signup`)
	assert.NoError(t, err)
}

func signupFinisher(db *sql.DB, mailAPI MailAPI, token string) *SignupFinisher {
	return &SignupFinisher{
		DB:      db,
		MailAPI: mailAPI,
		Token:   token,
	}
}

func signupStarter(db *sql.DB, mailAPI MailAPI, email string) *SignupStarter {
	return &SignupStarter{
		DB:      db,
		Email:   email,
		MailAPI: mailAPI,
	}
}

package command

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/brandur/passages-signup/mailclient"
	"github.com/brandur/passages-signup/testhelpers"
)

func TestSignupStarter(t *testing.T) {
	ctx := t.Context()

	// New signup
	t.Run("NewSignup", func(t *testing.T) {
		tx := testhelpers.TestTx(ctx, t)

		mailAPI := mailclient.NewFakeClient()
		mediator := signupStarter(mailAPI, testhelpers.TestEmail)

		res, err := mediator.Run(ctx, tx)
		require.NoError(t, err)

		require.False(t, res.ConfirmationRateLimited)
		require.False(t, res.ConfirmationResent)
		require.False(t, res.MaxNumAttempts)
		require.True(t, res.NewSignup)

		require.Len(t, mailAPI.MessagesSent, 1)
		require.Equal(t, testhelpers.TestEmail, mailAPI.MessagesSent[0].Recipient)
	})

	// Email already in progress, but with signup not completed
	t.Run("ConfirmationResent", func(t *testing.T) {
		tx := testhelpers.TestTx(ctx, t)

		// Manually insert a finished record
		_, err := tx.Exec(ctx, `
			INSERT INTO signup
				(email, token, last_sent_at)
			VALUES
				($1, 'not-a-real-token', NOW() - '1 month'::interval)
		`, testhelpers.TestEmail)
		require.NoError(t, err)

		mailAPI := mailclient.NewFakeClient()
		mediator := signupStarter(mailAPI, testhelpers.TestEmail)

		res, err := mediator.Run(ctx, tx)
		require.NoError(t, err)

		require.False(t, res.ConfirmationRateLimited)
		require.True(t, res.ConfirmationResent)
		require.False(t, res.MaxNumAttempts)
		require.False(t, res.NewSignup)

		require.Len(t, mailAPI.MessagesSent, 1)
		require.Equal(t, testhelpers.TestEmail, mailAPI.MessagesSent[0].Recipient)
	})

	// Email that's already subscribed (behaves identially to the case of
	// signup not completed above)
	t.Run("AlreadySubscribed", func(t *testing.T) {
		tx := testhelpers.TestTx(ctx, t)

		// Manually insert a finished record
		_, err := tx.Exec(ctx, `
                   INSERT INTO signup
                           (email, token, last_sent_at, completed_at)
                   VALUES
                           ($1, 'not-a-real-token', NOW() - '1 month'::interval, NOW())
           	`, testhelpers.TestEmail)
		require.NoError(t, err)

		mailAPI := mailclient.NewFakeClient()
		mediator := signupStarter(mailAPI, testhelpers.TestEmail)

		res, err := mediator.Run(ctx, tx)
		require.NoError(t, err)

		require.False(t, res.ConfirmationRateLimited)
		require.True(t, res.ConfirmationResent)
		require.False(t, res.MaxNumAttempts)
		require.False(t, res.NewSignup)

		require.Len(t, mailAPI.MessagesSent, 1)
		require.Equal(t, testhelpers.TestEmail, mailAPI.MessagesSent[0].Recipient)
	})

	// Email already in progress, but too soon after last attempt
	t.Run("ConfirmationRateLimited", func(t *testing.T) {
		tx := testhelpers.TestTx(ctx, t)

		// Manually insert a finished record
		_, err := tx.Exec(ctx, `
				INSERT INTO signup
					(email, token, last_sent_at)
				VALUES
					($1, 'not-a-real-token', NOW() - '1 hour'::interval)
			`, testhelpers.TestEmail)
		require.NoError(t, err)

		mailAPI := mailclient.NewFakeClient()
		mediator := signupStarter(mailAPI, testhelpers.TestEmail)

		res, err := mediator.Run(ctx, tx)
		require.NoError(t, err)

		require.True(t, res.ConfirmationRateLimited)
		require.False(t, res.ConfirmationResent)
		require.False(t, res.MaxNumAttempts)
		require.False(t, res.NewSignup)

		require.Empty(t, mailAPI.MessagesSent)
	})

	// We've tried to send a confirmation email many times before, but it's
	// never worked out so we give up.
	t.Run("MaxNumAttempts", func(t *testing.T) {
		tx := testhelpers.TestTx(ctx, t)

		// Manually insert a record at its maximum attempts
		numAttempts := maxNumSignupAttempts
		_, err := tx.Exec(ctx, `
			  	INSERT INTO signup
					  (email, token, num_attempts, last_sent_at)
				  VALUES
					  ($1, 'not-a-real-token', $2, NOW() - '1 month'::interval)
		  	`, testhelpers.TestEmail, numAttempts)
		require.NoError(t, err)

		mailAPI := mailclient.NewFakeClient()
		mediator := signupStarter(mailAPI, testhelpers.TestEmail)

		res, err := mediator.Run(ctx, tx)
		require.NoError(t, err)

		require.False(t, res.ConfirmationRateLimited)
		require.False(t, res.ConfirmationResent)
		require.True(t, res.MaxNumAttempts)
		require.False(t, res.NewSignup)

		require.Empty(t, mailAPI.MessagesSent)
	})

	// The exception to the case above is if the user has already completed the
	// signup flow. At that point, it doesn't matter what `num_attempts` is,
	// we'll still resend.
	t.Run("MaxNumAttemptsAlreadyCompleted", func(t *testing.T) {
		tx := testhelpers.TestTx(ctx, t)

		// Manually insert a record at its maximum attempts
		numAttempts := maxNumSignupAttempts
		_, err := tx.Exec(ctx, `
			  	INSERT INTO signup
					  (completed_at, email, token, num_attempts, last_sent_at)
				  VALUES
					  (NOW(), $1, 'not-a-real-token', $2, NOW() - '1 month'::interval)
		  	`, testhelpers.TestEmail, numAttempts)
		require.NoError(t, err)

		mailAPI := mailclient.NewFakeClient()
		mediator := signupStarter(mailAPI, testhelpers.TestEmail)

		res, err := mediator.Run(ctx, tx)
		require.NoError(t, err)

		require.False(t, res.ConfirmationRateLimited)
		require.True(t, res.ConfirmationResent)
		require.False(t, res.MaxNumAttempts)
		require.False(t, res.NewSignup)

		require.Len(t, mailAPI.MessagesSent, 1)
		require.Equal(t, testhelpers.TestEmail, mailAPI.MessagesSent[0].Recipient)
	})

	// Invalid email address
	t.Run("InvalidEmail", func(t *testing.T) {
		tx := testhelpers.TestTx(ctx, t)

		mailAPI := mailclient.NewFakeClient()
		mediator := signupStarter(mailAPI, "blah-not-an-email")

		_, err := mediator.Run(ctx, tx)
		require.ErrorIs(t, err, ErrInvalidEmail)
	})
}

//
// Private functions
//

func signupStarter(mailAPI mailclient.API, email string) *SignupStarter {
	return &SignupStarter{
		Email:          email,
		ListAddress:    testListAddress,
		MailAPI:        mailAPI,
		Renderer:       renderer,
		ReplyToAddress: testReplyToAddress,
	}
}

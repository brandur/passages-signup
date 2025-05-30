package command

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/brandur/passages-signup/mailclient"
	"github.com/brandur/passages-signup/testhelpers"
)

func TestSignupFinisher(t *testing.T) {
	const token = "test-token"

	ctx := t.Context()

	// Normal signup finish
	t.Run("FinishSignup", func(t *testing.T) {
		tx := testhelpers.TestTx(ctx, t)

		// Manually insert a record ready to be finished
		_, err := tx.Exec(ctx, `
				INSERT INTO signup
					(email, token)
				VALUES
					($1, $2)
			`, testhelpers.TestEmail, token)
		require.NoError(t, err)

		mailAPI := mailclient.NewFakeClient()
		mediator := signupFinisher(mailAPI, token)

		res, err := mediator.Run(ctx, tx)
		require.NoError(t, err)

		require.Equal(t, testhelpers.TestEmail, res.Email)
		require.True(t, res.SignupFinished)
		require.False(t, res.TokenNotFound)

		require.Len(t, mailAPI.MembersAdded, 1)
		require.Equal(t, testhelpers.TestEmail, mailAPI.MembersAdded[0].Email)

		//
		// Make sure it's idempotent by running it immediately again with the
		// same inputs
		//

		res, err = mediator.Run(ctx, tx)
		require.NoError(t, err)

		require.Equal(t, testhelpers.TestEmail, res.Email)
		require.True(t, res.SignupFinished)
		require.False(t, res.TokenNotFound)

		require.Len(t, mailAPI.MembersAdded, 2)
		require.Equal(t, testhelpers.TestEmail, mailAPI.MembersAdded[1].Email)
	})

	// Unknown token
	t.Run("UnknownToken", func(t *testing.T) {
		tx := testhelpers.TestTx(ctx, t)

		mailAPI := mailclient.NewFakeClient()
		mediator := signupFinisher(mailAPI, "not-a-token")

		res, err := mediator.Run(ctx, tx)
		require.NoError(t, err)

		require.Empty(t, res.Email)
		require.False(t, res.SignupFinished)
		require.True(t, res.TokenNotFound)

		require.Empty(t, mailAPI.MembersAdded)
	})
}

//
// Private functions
//

func signupFinisher(mailAPI mailclient.API, token string) *SignupFinisher {
	return &SignupFinisher{
		ListAddress: testListAddress,
		MailAPI:     mailAPI,
		Token:       token,
	}
}

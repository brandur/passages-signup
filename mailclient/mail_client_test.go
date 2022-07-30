package mailclient

import (
	"testing"

	"github.com/mailgun/mailgun-go/v3"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestInterpretMailgunError(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			"BuiltIn",
			xerrors.Errorf("test"),
			"test",
		},
		{
			"UnexpectedResponse",
			&mailgun.UnexpectedResponseError{Actual: 200, Data: []byte("test")},
			"Got unexpected status code 200 from Mailgun. Message: test",
		},
		{
			"UnexpectedResponseWithEmptyBody",
			&mailgun.UnexpectedResponseError{Actual: 200, Data: []byte("")},
			"Got unexpected status code 200 from Mailgun. Message: (empty)",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, interpretMailgunError(tc.err).Error())
		})
	}
}

package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mailgun/mailgun-go/v3"
	assert "github.com/stretchr/testify/require"
)

func TestInterpretMailgunError(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want error
	}{
		{
			"BuiltIn",
			fmt.Errorf("test"),
			errors.New("test"),
		},
		{
			"UnexpectedResponse",
			&mailgun.UnexpectedResponseError{Actual: 200, Data: []byte("test")},
			errors.New("Got unexpected status code 200 from Mailgun. Message: test"),
		},
		{
			"UnexpectedResponseWithEmptyBody",
			&mailgun.UnexpectedResponseError{Actual: 200, Data: []byte("")},
			errors.New("Got unexpected status code 200 from Mailgun. Message: (empty)"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, interpretMailgunError(tc.err))
		})
	}
}

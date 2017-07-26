package main

import (
	"fmt"
	"testing"

	assert "github.com/stretchr/testify/require"
	"gopkg.in/mailgun/mailgun-go.v1"
)

func TestInterpretMailgunError(t *testing.T) {
	{
		err := fmt.Errorf("test")
		assert.Equal(t, err.Error(), interpretMailgunError(err))
	}

	{
		err := &mailgun.UnexpectedResponseError{
			Actual: 200,
			Data:   []byte("test"),
		}
		assert.Equal(t,
			"Got unexpected status code 200 from Mailgun. Message: test",
			interpretMailgunError(err))
	}

	{
		err := &mailgun.UnexpectedResponseError{
			Actual: 200,
			Data:   []byte(""),
		}
		assert.Equal(t,
			"Got unexpected status code 200 from Mailgun. Message: (empty)",
			interpretMailgunError(err))
	}
}

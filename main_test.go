package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	assert "github.com/stretchr/testify/require"
	"gopkg.in/mailgun/mailgun-go.v1"
)

func TestHandleShow(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handleShow(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
}

func TestHandleSubmit(t *testing.T) {
	// Make sure that we're in testing so that we don't hit the actual Mailgun
	// API
	conf.PassagesEnv = envTesting

	{
		// does not respond to non-POST
		req := httptest.NewRequest("GET", "/submit", nil)
		w := httptest.NewRecorder()
		handleSubmit(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	}

	{
		// won't handle if the email parameter is missing
		req := httptest.NewRequest("POST", "/submit", nil)
		w := httptest.NewRecorder()
		handleSubmit(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	}

	{
		buf := bytes.NewBufferString("email=brandur@example.com")
		req := httptest.NewRequest("GET", "/", buf)
		w := httptest.NewRecorder()
		handleShow(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		_, err := ioutil.ReadAll(resp.Body)
		assert.NoError(t, err)
	}
}

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

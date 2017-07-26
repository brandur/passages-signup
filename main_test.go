package main

import (
	"bytes"
	"fmt"
	"io"
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

	testCases := []struct {
		name       string
		verb, path string
		body       io.Reader
		wantStatus int
	}{
		{
			"Renders",
			"POST", "/submit",
			bytes.NewBufferString("email=brandur@example.com"),
			http.StatusOK,
		},
		{
			"OnlyRespondsToPOST",
			"GET", "/submit",
			nil,
			http.StatusNotFound,
		},
		{
			"RequiresEmail",
			"POST", "/submit",
			nil,
			http.StatusUnprocessableEntity,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.verb, tc.path, tc.body)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			handleSubmit(w, req)

			resp := w.Result()
			assert.Equal(t, tc.wantStatus, resp.StatusCode)

			_, err := ioutil.ReadAll(resp.Body)
			assert.NoError(t, err)
		})
	}
}

func TestInterpretMailgunError(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			"BuiltIn",
			fmt.Errorf("test"),
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
			assert.Equal(t, tc.want, interpretMailgunError(tc.err))
		})
	}
}

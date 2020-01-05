package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brandur/passages-signup/testhelpers"
	"github.com/gorilla/mux"
	assert "github.com/stretchr/testify/require"
)

func init() {
	conf = Conf{
		AssetsDir:    "./",
		DatabaseURL:  testhelpers.DatabaseURL,
		NewsletterID: passagesID,

		// Make sure that we're in testing so that we don't hit the actual Mailgun
		// API
		PassagesEnv: envTesting,
	}
}

func TestHandleConfirm(t *testing.T) {
	db := testhelpers.ConnectDB(t)

	// Need to create a router so that path variables are processed correctly.
	router := mux.NewRouter()
	router.HandleFunc("/confirm/{token}", handleConfirm)

	token := "test-token"

	req := httptest.NewRequest("GET", "/confirm/"+token, nil)

	t.Run("FinishSignup", func(t *testing.T) {
		testhelpers.ResetDB(t, db)

		// Manually insert a record ready to be finished
		_, err := db.Exec(`
			INSERT INTO signup
				(email, token)
			VALUES
				($1, $2)
		`, testhelpers.TestEmail, token)
		assert.NoError(t, err)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		_, err = ioutil.ReadAll(resp.Body)
		assert.NoError(t, err)

		// Verify that the process has successfully transition the row's
		// `completed_at` to a non-nil value.
		var completedAt *time.Time
		err = db.QueryRow(`
			SELECT completed_at
			FROM signup
			WHERE token = $1
		`, token).Scan(&completedAt)
		assert.NoError(t, err)

		assert.NotNil(t, completedAt)
	})

	t.Run("UnknownToken", func(t *testing.T) {
		testhelpers.ResetDB(t, db)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		resp := w.Result()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		_, err := ioutil.ReadAll(resp.Body)
		assert.NoError(t, err)
	})
}

func TestHandleShow_Nanoglyph(t *testing.T) {
	conf.NewsletterID = nanoglyphID

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handleShow(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
}

func TestHandleShow_Passages(t *testing.T) {
	conf.NewsletterID = passagesID

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handleShow(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
}

func TestHandleSubmit(t *testing.T) {
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

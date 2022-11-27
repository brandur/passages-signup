package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/require"

	"github.com/brandur/passages-signup/db"
	"github.com/brandur/passages-signup/newslettermeta"
	"github.com/brandur/passages-signup/testhelpers"
)

func makeServer(ctx context.Context, t *testing.T, txStarter db.TXStarter, newsletterID string) *Server {
	t.Helper()

	s, err := NewServer(ctx, &Conf{
		DatabaseTXStarter: txStarter,
		MailgunAPIKey:     "fake-key",
		NewsletterID:      newsletterID,

		// Make sure that we're in testing so that we don't hit the actual Mailgun
		// API
		PassagesEnv: envTesting,

		Port:      "5001",
		PublicURL: testhelpers.TestPublicURL,
	})
	require.NoError(t, err)
	return s
}

func TestStaticAssets(t *testing.T) {
	setup := func(test func(*testing.T)) func(*testing.T) {
		return func(t *testing.T) {
			t.Helper()
			test(t)
		}
	}

	// Wraps the handler in a mux router for a more realistic simulation.
	wrapHandler := func(handler http.Handler) http.Handler {
		r := mux.NewRouter()
		r.PathPrefix("/public/").Handler(handler)
		return r
	}

	t.Run("Disk", setup(func(t *testing.T) { //nolint:thelper
		handler := wrapHandler(staticAssetsHandler(false))

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/public/tiny-preload-image.png", nil)
		handler.ServeHTTP(recorder, req)

		requireStatusOrPrintBody(t, http.StatusOK, recorder)
	}))

	t.Run("Embedded", setup(func(t *testing.T) { //nolint:thelper
		handler := wrapHandler(staticAssetsHandler(true))

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/public/tiny-preload-image.png", nil)
		handler.ServeHTTP(recorder, req)

		requireStatusOrPrintBody(t, http.StatusOK, recorder)
	}))
}

func TestHandleConfirm(t *testing.T) {
	var (
		ctx    context.Context
		router *mux.Router
		server *Server
		token  string
		tx     pgx.Tx
	)

	setup := func(test func(*testing.T)) func(*testing.T) {
		return func(t *testing.T) {
			t.Helper()
			ctx = context.Background()

			testhelpers.WithTestTransaction(ctx, t, func(testTx pgx.Tx) {
				server = makeServer(ctx, t, testTx, newslettermeta.PassagesID)
				token = "test-token"
				tx = testTx

				// Need to create a router so that path variables are processed correctly.
				router = mux.NewRouter()
				router.HandleFunc("/confirm/{token}", server.handleConfirm)

				test(t)
			})
		}
	}

	t.Run("FinishSignup", setup(func(t *testing.T) { //nolint:thelper
		// Manually insert a record ready to be finished
		_, err := tx.Exec(ctx, `
			INSERT INTO signup
				(email, token)
			VALUES
				($1, $2)
		`, testhelpers.TestEmail, token)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/confirm/"+token, nil)
		router.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Verify that the process has successfully transition the row's
		// `completed_at` to a non-nil value.
		var completedAt *time.Time
		err = tx.QueryRow(ctx, `
			SELECT completed_at
			FROM signup
			WHERE token = $1
		`, token).Scan(&completedAt)
		require.NoError(t, err)

		require.NotNil(t, completedAt)
	}))

	t.Run("UnknownToken", setup(func(t *testing.T) { //nolint:thelper
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/confirm/"+token, nil)
		router.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		require.Equal(t, http.StatusNotFound, resp.StatusCode)

		_, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
	}))
}

func TestHandleShow_DifferentNewsletters(t *testing.T) {
	var (
		ctx    context.Context
		server *Server
		tx     pgx.Tx
	)

	setup := func(test func(*testing.T)) func(*testing.T) {
		return func(t *testing.T) {
			t.Helper()
			ctx = context.Background()

			testhelpers.WithTestTransaction(ctx, t, func(testTx pgx.Tx) {
				tx = testTx

				test(t)
			})
		}
	}

	t.Run("NanoglyphSuccess", setup(func(t *testing.T) { //nolint:thelper
		server = makeServer(ctx, t, tx, newslettermeta.NanoglyphID)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		server.handleShow(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		_, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
	}))

	t.Run("PassagesSuccess", setup(func(t *testing.T) { //nolint:thelper
		server = makeServer(ctx, t, tx, newslettermeta.PassagesID)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		server.handleShow(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		_, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
	}))
}

func TestHandleSubmit(t *testing.T) {
	var (
		ctx    context.Context
		server *Server
	)

	setup := func(test func(*testing.T)) func(*testing.T) {
		return func(t *testing.T) {
			t.Helper()
			ctx = context.Background()

			testhelpers.WithTestTransaction(ctx, t, func(testTx pgx.Tx) {
				server = makeServer(ctx, t, testTx, newslettermeta.PassagesID)

				test(t)
			})
		}
	}

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
		t.Run(tc.name, setup(func(t *testing.T) { //nolint:thelper
			req := httptest.NewRequest(tc.verb, tc.path, tc.body)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			server.handleSubmit(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			require.Equal(t, tc.wantStatus, resp.StatusCode,
				fmt.Sprintf("Wrong status code (see above); body: %v", string(body)))
		}))
	}
}

func requireStatusOrPrintBody(t *testing.T, expectedStatusCode int, recorder *httptest.ResponseRecorder) {
	t.Helper()
	//nolint:bodyclose
	require.Equal(t, expectedStatusCode, recorder.Result().StatusCode,
		"Expected status %v, but got %v; body was: %s",
		expectedStatusCode,
		recorder.Result().StatusCode,
		recorder.Body.String(),
	)
}

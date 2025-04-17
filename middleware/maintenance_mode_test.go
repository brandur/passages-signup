package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/brandur/passages-signup/newslettermeta"
	"github.com/brandur/passages-signup/ptemplate"
)

func TestMaintenanceModeMiddlewareWrapper(t *testing.T) {
	var (
		handler  http.Handler
		renderer *ptemplate.Renderer
	)

	setup := func(test func(*testing.T)) func(*testing.T) {
		return func(t *testing.T) {
			t.Helper()

			handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("ok."))
			})

			var err error
			renderer, err = ptemplate.NewRenderer(&ptemplate.RendererConfig{
				DynamicReload:  true,
				NewsletterMeta: newslettermeta.MustMetaFor("list.brandur.org", newslettermeta.PassagesID),
				PublicURL:      "https://example.com",
				Templates:      os.DirFS("../"),
			})
			require.NoError(t, err)

			test(t)
		}
	}

	t.Run("MaintenanceOn", setup(func(t *testing.T) { //nolint:thelper
		handler = NewMaintenanceModeMiddleware(true, renderer).Wrapper(handler)

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
		handler.ServeHTTP(recorder, req)

		res := recorder.Result()
		require.Equal(t, http.StatusServiceUnavailable, res.StatusCode)

		data := recorder.Body.Bytes()
		require.Contains(t, string(data), "This application is currently in maintenance mode")
	}))

	t.Run("MaintenanceOff", setup(func(t *testing.T) { //nolint:thelper
		handler = NewMaintenanceModeMiddleware(false, renderer).Wrapper(handler)

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
		handler.ServeHTTP(recorder, req)

		requireStatusOrPrintBody(t, http.StatusOK, recorder)
	}))
}

func requireStatusOrPrintBody(t *testing.T, expectedStatusCode int, recorder *httptest.ResponseRecorder) {
	t.Helper()

	require.Equal(t, expectedStatusCode, recorder.Result().StatusCode,
		"Expected status %v, but got %v; body was: %s",
		expectedStatusCode,
		recorder.Result().StatusCode,
		recorder.Body.String(),
	)
}

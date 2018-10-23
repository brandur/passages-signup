package csrf

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	assert "github.com/stretchr/testify/require"
)

var testHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

func TestProtect(t *testing.T) {
	s := http.NewServeMux()
	s.HandleFunc("/", testHandler)

	// Success
	{
		p := Protect(AllowedOrigin("https://example.com"))(s)

		r, err := http.NewRequest("POST", "/", nil)
		assert.NoError(t, err)
		r.Header.Set("Origin", "https://example.com")

		rr := serveTestRequest(p, r)

		assert.Equal(t, http.StatusOK, rr.Code)
	}

	// Falls back to `Referer` if `Origin` isn't present
	{
		p := Protect(AllowedOrigin("https://example.com"))(s)

		r, err := http.NewRequest("POST", "/", nil)
		assert.NoError(t, err)
		r.Header.Set("Referer", "https://example.com/path/on/example")

		rr := serveTestRequest(p, r)

		assert.Equal(t, http.StatusOK, rr.Code)
	}

	// Safe methods (e.g. `GET`) are allowed
	{
		p := Protect(AllowedOrigin("https://example.com"))(s)

		r, err := http.NewRequest("GET", "/", nil)
		assert.NoError(t, err)
		r.Header.Set("Origin", "https://evil.com")

		rr := serveTestRequest(p, r)

		assert.Equal(t, http.StatusOK, rr.Code)
	}

	// Disallowed origin
	{
		p := Protect(AllowedOrigin("https://example.com"))(s)

		r, err := http.NewRequest("POST", "/", nil)
		assert.NoError(t, err)
		r.Header.Set("Origin", "https://evil.com")

		rr := serveTestRequest(p, r)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		data, err := ioutil.ReadAll(rr.Body)
		assert.NoError(t, err)
		assert.Equal(t,
			fmt.Sprintf("Forbidden - %s\n", ErrDisallowedOrigin),
			string(data))
	}

	// No `Origin` or `Referer`
	{
		p := Protect(AllowedOrigin("https://example.com"))(s)

		r, err := http.NewRequest("POST", "/", nil)
		assert.NoError(t, err)

		rr := serveTestRequest(p, r)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		data, err := ioutil.ReadAll(rr.Body)
		assert.NoError(t, err)
		assert.Equal(t,
			fmt.Sprintf("Forbidden - %s\n", ErrEmptyOrigin),
			string(data))
	}

	// Unparseable `Referer`
	{
		p := Protect(AllowedOrigin("https://example.com"))(s)

		r, err := http.NewRequest("POST", "/", nil)
		assert.NoError(t, err)
		r.Header.Set("Referer", "||https://example.com/path/on/example")

		rr := serveTestRequest(p, r)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		data, err := ioutil.ReadAll(rr.Body)
		assert.NoError(t, err)
		assert.Equal(t,
			fmt.Sprintf("Forbidden - %s\n", ErrInvalidReferer),
			string(data))
	}
}

func serveTestRequest(s http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, r)
	return rr
}

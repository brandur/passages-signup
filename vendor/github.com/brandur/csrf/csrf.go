package csrf

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

//
// Public
//

var (
	// ErrDisallowedOrigin is returned when the given Origin (or Referer) was
	// not in the allowed list.
	ErrDisallowedOrigin = fmt.Errorf("Origin invalid")

	// ErrEmptyOrigin is returned when we couldn't extract an origin from
	// either the Origin or Referer headers.
	ErrEmptyOrigin = fmt.Errorf("Origin empty")

	// ErrInvalidReferer is returned when the URL in the Referer header
	// couldn't be parsed.
	ErrInvalidReferer = fmt.Errorf("Referer URL not parseable")
)

// Option describes a functional option for configuring the CSRF handler.
type Option func(*csrf)

// AllowedOrigin configures an origin which is allowed for purposes of CSRF
// checking.
//
// Use a fully URL including scheme like `https://example.com` with no trailing
// slash..
func AllowedOrigin(origin string) Option {
	return func(cs *csrf) {
		cs.allowedOrigins = append(cs.allowedOrigins, origin)
	}
}

// ErrorHandler allows you to change the handler called when CSRF request
// processing encounters an invalid token or request. A typical use would be to
// provide a handler that returns a static HTML file with a HTTP 403 status. By
// default a HTTP 403 status and a plain text CSRF failure reason are served.
//
// Note that a custom error handler can also access the csrf.FailureReason(r)
// function to retrieve the CSRF validation reason from the request context.
func ErrorHandler(h http.Handler) Option {
	return func(cs *csrf) {
		cs.errorHandler = h
	}
}

// FailureReason makes CSRF validation errors available in the request context.
// This is useful when you want to log the cause of the error or report it to
// client.
func FailureReason(r *http.Request) error {
	if val, err := contextGet(r, errorKey{}); err == nil {
		if err, ok := val.(error); ok {
			return err
		}
	}

	return nil
}

// Protect wraps the given http.Handler with CSRF protection.
func Protect(opts ...Option) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		cs := parseOptions(h, opts...)

		if cs.errorHandler == nil {
			cs.errorHandler = http.HandlerFunc(unauthorizedHandler)
		}

		return cs
	}
}

// Implements http.Handler for the csrf type.
func (cs *csrf) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// HTTP methods not defined as idempotent ("safe") under RFC7231 require
	// inspection.
	if !contains(safeMethods, r.Method) {
		origin, err := originOrReferer(r)
		if err != nil {
			r = envError(r, ErrInvalidReferer)
			cs.errorHandler.ServeHTTP(w, r)
			return
		}

		if origin == "" {
			r = envError(r, ErrEmptyOrigin)
			cs.errorHandler.ServeHTTP(w, r)
			return
		}

		if !contains(cs.allowedOrigins, origin) {
			r = envError(r, ErrDisallowedOrigin)
			cs.errorHandler.ServeHTTP(w, r)
			return
		}
	}

	// Set the Vary: Cookie header to protect clients from caching the response.
	w.Header().Add("Vary", "Cookie")

	// Call the wrapped handler/router on success.
	cs.h.ServeHTTP(w, r)

	// Clear the request context after the handler has completed.
	contextClear(r)
}

//
// Private
//

type csrf struct {
	allowedOrigins []string
	errorHandler   http.Handler
	h              http.Handler
}

type errorKey struct{}

var (
	// Idempotent (safe) methods as defined by RFC7231 section 4.2.2.
	safeMethods = []string{"GET", "HEAD", "OPTIONS", "TRACE"}
)

// contains is a helper function to check if a string exists in a slice - e.g.
// whether a HTTP method exists in a list of safe methods.
func contains(vals []string, s string) bool {
	for _, v := range vals {
		if v == s {
			return true
		}
	}

	return false
}

func contextClear(r *http.Request) {
	// no-op for go1.7+
}

func contextGet(r *http.Request, key interface{}) (interface{}, error) {
	val := r.Context().Value(key)
	if val == nil {
		return nil, fmt.Errorf("no value exists in the context for key %q", key)
	}

	return val, nil
}

func contextSave(r *http.Request, key, val interface{}) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, key, val)
	return r.WithContext(ctx)
}

// envError stores a CSRF error in the request context.
func envError(r *http.Request, err error) *http.Request {
	return contextSave(r, errorKey{}, err)
}

func originOrReferer(r *http.Request) (string, error) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		return origin, nil
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		return "", nil
	}

	u, err := url.Parse(referer)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s://%s", u.Scheme, u.Host), nil
}

// parseOptions parses the supplied options functions and returns a configured
// csrf handler.
func parseOptions(h http.Handler, opts ...Option) *csrf {
	cs := &csrf{
		h: h,
	}

	// Range over each options function and apply it to our csrf type to
	// configure it. Options functions are applied in order, with any
	// conflicting options overriding earlier calls.
	for _, option := range opts {
		option(cs)
	}

	return cs
}

// unauthorizedhandler sets a HTTP 403 Forbidden status and writes the
// CSRF failure reason to the response.
func unauthorizedHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, fmt.Sprintf("%s - %s",
		http.StatusText(http.StatusForbidden), FailureReason(r)),
		http.StatusForbidden)
	return
}

# Go stateless CSRF [![Build Status](https://travis-ci.org/brandur/csrf.svg?branch=master)](https://travis-ci.org/brandur/csrf)

A stateless CSRF middleware for Go. It works by relying the
presence of the [`Origin`][origin] header. It will also
fall back to `Referer` if that's provided and `Origin`
isn't, but it has no mechanic for embedding a form token.

``` go
import (
    "github.com/brandur/csrf"
)

func main() {
    var handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
    handler = csrf.Protect(csrf.AllowedOrigin("https://example.com"))(s)

    ...
}
```

[origin]: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Origin

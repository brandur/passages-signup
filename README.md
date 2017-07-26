# passages-signup [![Build Status](https://travis-ci.org/brandur/passages-signup.svg?branch=master)](https://travis-ci.org/brandur/passages-signup)

This is a basic signup form for my mailing list _Passages &
Glass_. My site is otherwise statically hosted, so this
provides a basic backend that can talk to the Mailgun API.

## Setup

Clone, configure, install, and run:

``` sh
go get -u github.com/brandur/passages-signup
cd $GOPATH/src/github.com/brandur/passages-signup
cp .env.sample .env
# edit CSRF_SECRET and MAILGUN_API_KEY in .env
go install && forego start -p 5001 web
```

Open your browser to [localhost:5001](http://localhost:5001).

## Vendoring Dependencies

Dependencies are managed with govendor. New ones can be
vendored using these commands:

    go get -u github.com/kardianos/govendor
    govendor add +external

## Operations

Hosted on Heroku at `passages-signup`.

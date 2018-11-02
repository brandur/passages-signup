# passages-signup [![Build Status](https://travis-ci.org/brandur/passages-signup.svg?branch=master)](https://travis-ci.org/brandur/passages-signup)

A basic backend for the signup form of my newsletter
_Passages & Glass_. The rest of its site is statically
hosted, so this program is designed to hold an API key and
talk to the Mailgun API.

## Setup

Clone, configure, install, and run:

``` sh
go get -u github.com/brandur/passages-signup

cd $GOPATH/src/github.com/brandur/passages-signup

cp .env.sample .env

# open .env; edit MAILGUN_API_KEY

go install && forego start -p 5001 web
```

Open your browser to [localhost:5001](http://localhost:5001).

## Testing

    createdb passages-signup-test
    psql passages-signup-test < schema.sql

## Vendoring Dependencies

Dependencies are managed with dep. New ones can be vendored
using these commands:

    dep ensure -add github.com/foo/bar

## Operations

Hosted on Heroku at [`passages-signup`][heroku].

[heroku]: https://passages-signup.herokuapp.com

# passages-signup [![Build Status](https://github.com/brandur/passages-signup/workflows/passages-signup%20CI/badge.svg)](https://github.com/brandur/passages-signup/actions)


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
createdb passages-signup
psql passages-signup < schema.sql

# open .env; edit MAILGUN_API_KEY

go install && forego start -p 5001 web
```

Open your browser to [localhost:5001](http://localhost:5001).

## Testing

    createdb passages-signup-test
    psql passages-signup-test < schema.sql

## Operations

Hosted on Heroku at [`passages-signup`][heroku].

## Heroku deploy

    heroku addons:add heroku-postgresql:hobby-dev -r heroku-nanoglyph
    heroku pg:psql -r heroku-nanoglyph < schema.sql
    git push heroku master

[heroku]: https://passages-signup.herokuapp.com

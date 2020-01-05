# passages-signup [![Build Status](https://github.com/brandur/passages-signup/workflows/passages-signup%20CI/badge.svg)](https://github.com/brandur/passages-signup/actions)


A backend for the signup forms of my newsletters _Nanoglyph_ and _Passages & Glass_. Tracks email address through the process of confirming them, then adds them to a list through Mailgun's API.

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

* Cloud Run service [`nanoglyph-signup`](https://nanoglyph-signup-5slhbjdbla-uc.a.run.app/) (project `passages-signup`).
* Cloud Run service [`passages-signup`](https://passages-signup-5slhbjdbla-uc.a.run.app/) (project `passages-signup`).
* Heroku app [`nanoglyph-signup`](https://nanoglyph-signup.herokuapp.com).
* Heroku app [`passages-signup`](https://passages-signup.herokuapp.com).

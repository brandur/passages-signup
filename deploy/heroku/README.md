# Heroku Deployment

Add a database:

    heroku addons:add heroku-postgresql:hobby-dev -r heroku-nanoglyph
    heroku pg:psql -r heroku-nanoglyph < schema.sql

Assign remotes:

    heroku git:remote -r heroku-nanoglyph -a nanoglyph-signup
    heroku git:remote -r heroku-passages -a passages-signup

Push code:

    git push heroku-nanoglyph master
    git push heroku-passages master

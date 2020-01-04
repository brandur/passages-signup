# Heroku Deployment

Add a database:

    heroku addons:add heroku-postgresql:hobby-dev -r heroku-nanoglyph
    heroku pg:psql -r heroku-nanoglyph < schema.sql

Push code:

    git push heroku-nanoglyph master
    git push heroku-passages master

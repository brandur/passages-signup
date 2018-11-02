BEGIN;

DROP TABLE IF EXISTS signup;

CREATE TABLE signup (
    id           BIGSERIAL    PRIMARY KEY,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    email        VARCHAR(500) NOT NULL UNIQUE,
    last_sent_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    token        VARCHAR(100) NOT NULL UNIQUE
);

CREATE UNIQUE INDEX signup_token
    ON signup (token);

COMMIT;

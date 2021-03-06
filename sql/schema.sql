BEGIN;

DROP TABLE IF EXISTS signup;

CREATE TABLE signup (
    id           BIGSERIAL    PRIMARY KEY,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    email        VARCHAR(500) NOT NULL UNIQUE,
    last_sent_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    num_attempts BIGINT       NOT NULL DEFAULT 1,
    token        VARCHAR(100) NOT NULL UNIQUE
);

CREATE UNIQUE INDEX signup_email
    ON signup (email);

CREATE INDEX signup_last_sent_at
    ON signup (last_sent_at)
    WHERE last_sent_at IS NOT NULL;

CREATE UNIQUE INDEX signup_token
    ON signup (token)
    WHERE token IS NOT NULL;

COMMIT;

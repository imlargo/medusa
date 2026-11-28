-- +goose Up
-- Refresh tokens, stored by the SHA-256 of their value. A family is one
-- sign-in and every token rotated from it.
CREATE TABLE refresh_tokens (
    hash       text PRIMARY KEY,
    family     text        NOT NULL,
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX refresh_tokens_family ON refresh_tokens (family);
CREATE INDEX refresh_tokens_expires_at ON refresh_tokens (expires_at);

-- +goose Down
DROP TABLE refresh_tokens;

-- +goose Up
-- The token a refresh token replaced. Tokens with the same parent are
-- siblings, issued when one token is presented several times at once.
ALTER TABLE refresh_tokens ADD COLUMN parent text;

CREATE INDEX refresh_tokens_parent ON refresh_tokens (parent) WHERE parent IS NOT NULL;

-- +goose Down
DROP INDEX refresh_tokens_parent;
ALTER TABLE refresh_tokens DROP COLUMN parent;

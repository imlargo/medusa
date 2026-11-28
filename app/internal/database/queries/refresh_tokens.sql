-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (hash, family, user_id, expires_at)
VALUES ($1, $2, $3, $4);

-- name: ConsumeRefreshToken :one
-- Marks the token as used and says whether it already was, in one statement:
-- the row lock makes two requests with the same token take turns.
WITH prior AS (
    SELECT hash, used_at FROM refresh_tokens WHERE refresh_tokens.hash = $1 FOR UPDATE
)
UPDATE refresh_tokens t
SET used_at = COALESCE(t.used_at, now())
FROM prior
WHERE t.hash = prior.hash
RETURNING t.hash, t.family, t.user_id, t.expires_at, (prior.used_at IS NOT NULL)::boolean AS reused;

-- name: DeleteRefreshTokenFamily :exec
DELETE FROM refresh_tokens
WHERE family = $1;

-- name: DeleteExpiredRefreshTokens :execrows
DELETE FROM refresh_tokens
WHERE expires_at < now();

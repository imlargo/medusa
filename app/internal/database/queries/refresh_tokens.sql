-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (hash, family, parent, user_id, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ConsumeRefreshToken :one
-- Marks the token as used and says whether it already was, and how long ago,
-- in one statement: the row lock makes two requests with the same token take
-- turns. On a first use it also retires the token's unused siblings, so only
-- one of the tokens issued for the same parent continues.
WITH prior AS (
    SELECT hash, parent, used_at FROM refresh_tokens WHERE refresh_tokens.hash = $1 FOR UPDATE
),
retired AS (
    UPDATE refresh_tokens s
    SET used_at = now()
    FROM prior
    WHERE prior.used_at IS NULL
      AND s.parent = prior.parent
      AND s.hash <> prior.hash
      AND s.used_at IS NULL
)
UPDATE refresh_tokens t
SET used_at = COALESCE(t.used_at, now())
FROM prior
WHERE t.hash = prior.hash
RETURNING t.hash, t.family, t.parent, t.user_id, t.expires_at,
          (prior.used_at IS NOT NULL)::boolean AS reused,
          COALESCE(EXTRACT(EPOCH FROM now() - prior.used_at), 0)::double precision AS used_ago_seconds;

-- name: DeleteRefreshTokenFamily :exec
DELETE FROM refresh_tokens
WHERE family = $1;

-- name: DeleteExpiredRefreshTokens :execrows
DELETE FROM refresh_tokens
WHERE expires_at < now();

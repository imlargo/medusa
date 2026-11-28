-- The flag store's schema. Every statement is idempotent, so Install can run
-- on every deploy.

CREATE TABLE IF NOT EXISTS medusa_flags (
    name       text PRIMARY KEY,
    enabled    boolean     NOT NULL DEFAULT false,
    percentage integer     NOT NULL DEFAULT 0 CHECK (percentage BETWEEN 0 AND 100),
    allow      text[]      NOT NULL DEFAULT '{}',
    updated_at timestamptz NOT NULL DEFAULT now()
);

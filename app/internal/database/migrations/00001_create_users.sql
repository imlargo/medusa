-- +goose Up
-- Same shape as the table GORM's AutoMigrate created, and IF NOT EXISTS, so a
-- database that already has it adopts this version without a change.
CREATE TABLE IF NOT EXISTS users (
    id         bigserial PRIMARY KEY,
    created_at timestamptz,
    updated_at timestamptz,
    email      text NOT NULL,
    password   text NOT NULL,
    CONSTRAINT uni_users_email UNIQUE (email)
);

-- No Down: on a database that adopted this baseline, rolling it back would
-- drop every account.

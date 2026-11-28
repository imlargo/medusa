// Package flagstore keeps feature flags in Postgres, so they change with a
// statement and not with a deploy. It is the store behind the core's flags
// package:
//
//	store := flagstore.New(pool)
//	features := flags.New(func(ctx context.Context) (map[string]flags.Flag, error) {
//	    stored, err := store.Load(ctx)
//	    if err != nil {
//	        return nil, err
//	    }
//	    out := make(map[string]flags.Flag, len(stored))
//	    for name, flag := range stored {
//	        out[name] = flags.Flag(flag)
//	    }
//	    return out, nil
//	})
//
// Its Flag has the fields of flags.Flag, so one converts to the other; this
// module does not import the core.
//
// The store owns one table, medusa_flags. [Install] creates it, next to the
// application's migrations. A flag is changed with [Store.Set], or with SQL:
//
//	UPDATE medusa_flags SET percentage = 25 WHERE name = 'new_checkout';
//
// Every process sees the change at its next refresh.
package flagstore

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/imlargo/medusa/postgres"
)

//go:embed schema.sql
var schema string

// installLock serializes Install across the replicas of a deploy.
const installLock = "medusa_flags_install"

// Install creates the flags table. Every statement is idempotent: run it on
// every deploy, in the migration step.
func Install(ctx context.Context, conn postgres.Conn) error {
	err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", installLock); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, schema)
		return err
	})
	if err != nil {
		return fmt.Errorf("flagstore: install schema: %w", err)
	}
	return nil
}

// Flag is one feature's rollout, as stored. Its fields are those of the
// core's flags.Flag.
type Flag struct {
	// Enabled turns the flag on for everyone.
	Enabled bool

	// Percentage of keys the flag is on for, from 0 to 100.
	Percentage int

	// Allow lists keys the flag is always on for.
	Allow []string
}

// Store reads and writes flags.
type Store struct {
	conn postgres.Conn
}

// New keeps flags in the database behind conn.
func New(conn postgres.Conn) *Store {
	return &Store{conn: conn}
}

// Load returns every flag, by name.
func (s *Store) Load(ctx context.Context) (map[string]Flag, error) {
	rows, err := s.conn.Query(ctx, "SELECT name, enabled, percentage, allow FROM medusa_flags")
	if err != nil {
		return nil, fmt.Errorf("flagstore: load: %w", err)
	}
	defer rows.Close()

	flags := map[string]Flag{}
	for rows.Next() {
		var (
			name string
			flag Flag
		)
		if err := rows.Scan(&name, &flag.Enabled, &flag.Percentage, &flag.Allow); err != nil {
			return nil, fmt.Errorf("flagstore: load: %w", err)
		}
		flags[name] = flag
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("flagstore: load: %w", err)
	}
	return flags, nil
}

// Set creates the flag name or replaces its rollout. A percentage outside
// 0-100 is an error.
func (s *Store) Set(ctx context.Context, name string, flag Flag) error {
	if name == "" {
		return errors.New("flagstore: set: the flag has no name")
	}
	if flag.Percentage < 0 || flag.Percentage > 100 {
		return fmt.Errorf("flagstore: set %s: percentage %d is outside 0-100", name, flag.Percentage)
	}
	allow := flag.Allow
	if allow == nil {
		allow = []string{}
	}

	_, err := s.conn.Exec(ctx, `
		INSERT INTO medusa_flags (name, enabled, percentage, allow)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (name) DO UPDATE
		SET enabled = EXCLUDED.enabled, percentage = EXCLUDED.percentage, allow = EXCLUDED.allow, updated_at = now()`,
		name, flag.Enabled, flag.Percentage, allow)
	if err != nil {
		return fmt.Errorf("flagstore: set %s: %w", name, err)
	}
	return nil
}

// Delete removes the flag name: it is then off for everyone, like any flag
// that is not defined. Deleting one that does not exist is not an error.
func (s *Store) Delete(ctx context.Context, name string) error {
	if _, err := s.conn.Exec(ctx, "DELETE FROM medusa_flags WHERE name = $1", name); err != nil {
		return fmt.Errorf("flagstore: delete %s: %w", name, err)
	}
	return nil
}

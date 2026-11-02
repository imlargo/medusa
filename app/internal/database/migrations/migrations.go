// Package migrations holds the goose migrations: the schema, versioned. They
// are also the schema sqlc generates the queries from, so the two cannot
// drift.
package migrations

import "embed"

// FS holds every migration, embedded in the binaries that apply them.
//
//go:embed *.sql
var FS embed.FS

package session

import "embed"

// Migrations are the migrations of the tables this feature owns:
// refresh_tokens.
//
//go:embed migrations/*.sql
var Migrations embed.FS

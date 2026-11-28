package user

import "embed"

// Migrations are the migrations of the tables this feature owns: users.
//
//go:embed migrations/*.sql
var Migrations embed.FS

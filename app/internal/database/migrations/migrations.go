// Package migrations is the database's one migration history, assembled
// from the migrations each feature owns. goose applies them in the order of
// the version their file names start with, whichever feature they live in.
//
// A feature that gains its first table is added here.
package migrations

import (
	"github.com/imlargo/medusa/postgres"

	"github.com/imlargo/medusa/app/internal/session"
	"github.com/imlargo/medusa/app/internal/user"
)

// FS holds every migration of every feature.
var FS = postgres.MustMergeMigrations(
	user.Migrations,
	session.Migrations,
)

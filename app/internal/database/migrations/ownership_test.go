package migrations_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/imlargo/medusa/app/internal/database/migrations"
)

// featuresDir is app/internal, where each feature is a directory.
const featuresDir = "../.."

var (
	sqlComment  = regexp.MustCompile(`--[^\n]*`)
	createTable = regexp.MustCompile(`(?i)\bcreate\s+table\s+(?:if\s+not\s+exists\s+)?([a-z_][a-z0-9_]*)`)
	writesTable = regexp.MustCompile(`(?i)\b(?:insert\s+into|update|delete\s+from)\s+([a-z_][a-z0-9_]*)`)
)

// sqlFiles returns the .sql files of every feature's dir ("migrations" or
// "queries"), by feature.
func sqlFiles(t *testing.T, dir string) map[string][]string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(featuresDir, "*", dir, "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	byFeature := map[string][]string{}
	for _, path := range paths {
		feature := filepath.Base(filepath.Dir(filepath.Dir(path)))
		byFeature[feature] = append(byFeature[feature], path)
	}
	return byFeature
}

func readSQL(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sqlComment.ReplaceAllString(string(content), "")
}

// tableOwners maps each table to the feature whose migrations create it.
func tableOwners(t *testing.T) map[string]string {
	t.Helper()

	owners := map[string]string{}
	for feature, paths := range sqlFiles(t, "migrations") {
		for _, path := range paths {
			for _, match := range createTable.FindAllStringSubmatch(readSQL(t, path), -1) {
				table := strings.ToLower(match[1])
				if other, taken := owners[table]; taken && other != feature {
					t.Errorf("table %s is created by both %s and %s: a table has one owner", table, other, feature)
				}
				owners[table] = feature
			}
		}
	}
	return owners
}

// A table has one owner, the feature whose migrations create it, and only
// the owner writes it. Another feature that needs the data changed calls the
// owner's Go API; one that needs to read it lists the owner's schema in
// sqlc.yaml. Without this check the boundary is a convention, and a query
// that updates another feature's table compiles like any other.
func TestOnlyATablesOwnerWritesIt(t *testing.T) {
	owners := tableOwners(t)

	for feature, paths := range sqlFiles(t, "queries") {
		for _, path := range paths {
			for _, match := range writesTable.FindAllStringSubmatch(readSQL(t, path), -1) {
				table := strings.ToLower(match[1])
				if table == "set" { // ON CONFLICT ... DO UPDATE SET
					continue
				}
				owner, known := owners[table]
				switch {
				case !known:
					t.Errorf("%s writes %s, which no feature's migrations create", path, table)
				case owner != feature:
					t.Errorf("%s writes %s, which %s owns: call %s's API instead", path, table, owner, owner)
				}
			}
		}
	}
}

// A feature's migrations only run if the feature is listed in FS. One that
// is left out compiles, passes its unit tests on a fake, and has no tables
// in production.
func TestEveryFeaturesMigrationsAreInTheHistory(t *testing.T) {
	merged, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	inHistory := map[string]bool{}
	for _, name := range merged {
		inHistory[name] = true
	}

	for feature, paths := range sqlFiles(t, "migrations") {
		for _, path := range paths {
			if !inHistory[filepath.Base(path)] {
				t.Errorf("%s is not in migrations.FS: add %s.Migrations to it", path, feature)
			}
		}
	}
}

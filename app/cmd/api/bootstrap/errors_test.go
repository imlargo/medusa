package bootstrap

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imlargo/medusa/apperr"
	"github.com/imlargo/medusa/httperr"
)

var updateCatalog = flag.Bool("update", false, "rewrite api/errors.md from the declared errors")

// markdownEscaper keeps a message from breaking the table or being read as HTML.
var markdownEscaper = strings.NewReplacer("|", `\|`, "<", "&lt;", ">", "&gt;")

// catalogPath is the API's error catalog, relative to this package.
var catalogPath = filepath.Join("..", "..", "..", "api", "errors.md")

// The bootstrap package links every feature, so every error the API can
// answer with is declared by the time this test runs. Codes are the contract
// clients branch on: two declarations of one code would match each other's
// errors, and a code missing from the catalog is undocumented.
func TestErrorCatalog(t *testing.T) {
	declared := apperr.Declared()

	var b strings.Builder
	b.WriteString("# Error codes\n\n")
	b.WriteString("Every error the API answers with, as the `code` of its RFC 9457 problem. Generated from the declared errors; regenerate with `go test ./cmd/api/bootstrap -run TestErrorCatalog -update`.\n\n")
	b.WriteString("| Code | Status | Kind | Message |\n|---|---|---|---|\n")

	seen := make(map[string]bool, len(declared))
	for _, e := range declared {
		if seen[e.Code()] {
			t.Errorf("code %q is declared more than once", e.Code())
		}
		seen[e.Code()] = true
		fmt.Fprintf(&b, "| `%s` | %d | %s | %s |\n", e.Code(), httperr.Status(e.Kind()), e.Kind(), markdownEscaper.Replace(e.Message()))
	}
	want := b.String()

	if *updateCatalog {
		if err := os.WriteFile(catalogPath, []byte(want), 0o600); err != nil {
			t.Fatalf("write catalog: %v", err)
		}
		return
	}

	got, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("read catalog: %v (run with -update to create it)", err)
	}
	if string(got) != want {
		t.Errorf("%s is out of date with the declared errors; run go test ./cmd/api/bootstrap -run TestErrorCatalog -update", catalogPath)
	}
}

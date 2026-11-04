package apperr

import (
	"cmp"
	"slices"
	"sync"
)

// declared records every distinct error New has built, so a program can list
// the errors it can answer with: the error catalog of an API.
var declared struct {
	sync.Mutex
	seen   map[declaration]bool
	errors []*Error
}

type declaration struct {
	kind    Kind
	code    string
	message string
}

func register(e *Error) {
	d := declaration{kind: e.kind, code: e.code, message: e.message}

	declared.Lock()
	defer declared.Unlock()
	if declared.seen[d] {
		return
	}
	if declared.seen == nil {
		declared.seen = make(map[declaration]bool)
	}
	declared.seen[d] = true
	declared.errors = append(declared.errors, e)
}

// Declared returns every error declared so far in the program, sorted by
// code. Package-level declarations run before main, so in a test or a binary
// that links every feature this is the complete list of codes the API can
// answer with.
//
// Two declarations with the same code both appear. Since Is compares codes,
// they would match each other's errors: a catalog test should fail on them.
func Declared() []*Error {
	declared.Lock()
	defer declared.Unlock()

	out := slices.Clone(declared.errors)
	slices.SortStableFunc(out, func(a, b *Error) int { return cmp.Compare(a.code, b.code) })
	return out
}

package api

import (
	"encoding/base64"
	"encoding/json"
	"reflect"

	"github.com/imlargo/medusa/apperr"
)

// Page limits, the same for every list of an API.
const (
	// DefaultLimit is how many items a list returns when the client does
	// not say.
	DefaultLimit = 20

	// MaxLimit is the most a client may ask for.
	MaxLimit = 100
)

// ErrInvalidCursor: the cursor is not one this API issued, or it was issued
// for another list.
var ErrInvalidCursor = apperr.Invalid("request.invalid_cursor", "the cursor is not valid: start again without one")

// Page is the query of a list. Embed it in the input, so every list of the
// API pages with the same two parameters and the same limits:
//
//	type ListInput struct {
//	    api.Page
//	    Status string `query:"status" enum:"open,closed"`
//	}
type Page struct {
	Cursor string `query:"cursor" doc:"The next_cursor of the previous page. Leave it out for the first page."`
	Limit  int    `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"How many items to return"`
}

// List is what a list answers with: a page of items, and the cursor of the
// next page while there is one.
type List[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty" doc:"Pass it as cursor to get the next page. Absent on the last page."`
}

// NewList builds a page from the rows a query returned when it was asked for
// one more than limit. That extra row tells whether a next page exists
// without counting; it is dropped, and the cursor is made from the last row
// kept, by position: what the next query must start after (an ID, or a
// timestamp and an ID).
//
//	rows, err := repo.ListAfter(ctx, after, in.Limit+1)
//	return api.NewList(rows, in.Limit, func(o Order) any { return o.ID }), nil
func NewList[T any](rows []T, limit int, position func(last T) any) *List[T] {
	list := &List[T]{Items: rows}
	if list.Items == nil {
		list.Items = []T{} // "items": [] and never null
	}
	if limit > 0 && len(rows) > limit {
		list.Items = rows[:limit]
		list.NextCursor = EncodeCursor(position(list.Items[limit-1]))
	}
	return list
}

// EncodeCursor makes an opaque cursor out of a position. Clients treat it
// as a token: what is inside can change without breaking them.
func EncodeCursor(position any) string {
	encoded, err := json.Marshal(position)
	if err != nil {
		// A position is an ID or a small struct of them. One that cannot be
		// encoded is a programming error.
		panic("api: a cursor position that cannot be encoded as JSON: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

// DecodeCursor reads a cursor back into position, a pointer. An empty
// cursor is the first page: position is left as it is and the result is
// false. A cursor that cannot be read is [ErrInvalidCursor].
func DecodeCursor(cursor string, position any) (bool, error) {
	if cursor == "" {
		return false, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return false, ErrInvalidCursor.WithCause(err)
	}
	if err := json.Unmarshal(decoded, position); err != nil {
		return false, ErrInvalidCursor.WithCause(err)
	}
	return true, nil
}

// reflectType is reflect.TypeFor, named for what it is used for here.
func reflectType[T any]() reflect.Type {
	return reflect.TypeFor[T]()
}

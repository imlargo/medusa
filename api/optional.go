package api

import (
	"bytes"
	"encoding/json"

	"github.com/danielgtaylor/huma/v2"
)

// Optional is a body field of a partial update, where three things have to
// be told apart: the field was not sent (leave it), it was sent as null
// (clear it), or it was sent with a value (set it). A plain pointer cannot:
// absent and null both decode to nil.
//
//	type UpdateInput struct {
//	    ID   int64 `path:"id"`
//	    Body struct {
//	        Name  api.Optional[string] `json:"name,omitzero"`
//	        Notes api.Optional[string] `json:"notes,omitzero"`
//	    }
//	}
//
//	if in.Body.Notes.Sent {
//	    if in.Body.Notes.Null { clear } else { set in.Body.Notes.Value }
//	}
//
// Tag it omitzero, so the field is not required and an unsent one is left
// out when the type is written back as JSON.
type Optional[T any] struct {
	// Sent is true when the field was in the body, as a value or as null.
	Sent bool

	// Null is true when it was sent as null.
	Null bool

	// Value is what was sent, when it was not null.
	Value T
}

// Get returns the value and whether there is one: the field was sent, and
// not as null.
func (o Optional[T]) Get() (T, bool) {
	return o.Value, o.Sent && !o.Null
}

// UnmarshalJSON runs only for a field present in the body.
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Sent = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		o.Null = true
		return nil
	}
	return json.Unmarshal(data, &o.Value)
}

// MarshalJSON writes the value, or null.
func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if !o.Sent || o.Null {
		return []byte("null"), nil
	}
	return json.Marshal(o.Value)
}

// Schema describes the field in the document as a T that may be null.
func (o Optional[T]) Schema(r huma.Registry) *huma.Schema {
	schema := *huma.SchemaFromType(r, reflectType[T]())
	schema.Nullable = true
	return &schema
}

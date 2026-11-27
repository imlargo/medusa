// Package reqid carries the request ID in a context.Context. It exists so
// packages that must not import Gin (httpclient) can read what the medusa
// package and its middleware write.
package reqid

import "context"

type key struct{}

// With returns ctx carrying id.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, key{}, id)
}

// From returns the ID ctx carries, or "".
func From(ctx context.Context) string {
	id, _ := ctx.Value(key{}).(string)
	return id
}

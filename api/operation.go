package api

import (
	"context"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa/apperr"
	"github.com/imlargo/medusa/httperr"
)

// Empty is the result of a handler that answers with no body:
//
//	func (h *Handler) Delete(ctx context.Context, in *DeleteInput) (*api.Empty, error) {
//	    return nil, h.orders.Delete(ctx, in.ID)
//	}
//
// Such a route answers 204 unless [Status] says otherwise.
type Empty struct{}

// OperationOption configures one route.
type OperationOption func(*operation)

type operation struct {
	huma   huma.Operation
	errors []*apperr.Error
	guards []gin.HandlerFunc
	scopes []string
}

// Summary is the route's one-line title in the document. Without it, one is
// made from the method and the path.
func Summary(summary string) OperationOption {
	return func(o *operation) { o.huma.Summary = summary }
}

// Description explains the route in the document. It may use Markdown.
func Description(description string) OperationOption {
	return func(o *operation) { o.huma.Description = description }
}

// Status sets the status of a successful answer: 201 for a route that
// creates, 202 for one that accepts work to do later. The default is 200,
// or 204 for a handler that returns [Empty].
func Status(status int) OperationOption {
	return func(o *operation) { o.huma.DefaultStatus = status }
}

// Errors lists the application errors the route can answer with. Each one
// is documented under its status, with its code, so a client reads from the
// document what it has to handle. Validation errors and server.internal are
// always listed.
func Errors(errs ...*apperr.Error) OperationOption {
	return func(o *operation) { o.errors = append(o.errors, errs...) }
}

// Scopes requires the caller to have scopes, and documents them on the
// route's security. guard builds the middleware that checks them; with the
// auth battery it is auth.Require:
//
//	api.Post(v1, "/orders", h.Create, api.Scopes(auth.Require, "orders:write"))
func Scopes(guard func(scopes ...string) gin.HandlerFunc, scopes ...string) OperationOption {
	return func(o *operation) {
		o.scopes = append(o.scopes, scopes...)
		o.guards = append(o.guards, guard(scopes...))
	}
}

// Idempotent documents that the route takes an Idempotency-Key header: a
// client that sends the same key again gets the first answer instead of a
// second effect. Declaring it lets clients send the key from the first
// version of the API; the handler reads it from a header field of its input.
func Idempotent() OperationOption {
	return func(o *operation) {
		o.huma.Parameters = append(o.huma.Parameters, &huma.Param{
			Name:        "Idempotency-Key",
			In:          "header",
			Description: "A key unique to this request. Sending it again returns the first answer instead of repeating the effect.",
			Schema:      &huma.Schema{Type: "string", MaxLength: new(255)},
		})
	}
}

// Hidden leaves the route out of the document. It works like any other.
func Hidden() OperationOption {
	return func(o *operation) { o.huma.Hidden = true }
}

// MaxBody sets the largest request body the route reads, 1 MiB by default.
func MaxBody(bytes int64) OperationOption {
	return func(o *operation) { o.huma.MaxBodyBytes = bytes }
}

// Get registers a route that reads. See [Post] for how the handler's types
// are used.
func Get[I, O any](g *Group, path string, handler func(context.Context, *I) (*O, error), opts ...OperationOption) {
	register(g, http.MethodGet, path, handler, opts)
}

// Post registers a route. I is the input: its fields are bound from the
// path, the query, the headers and the body by their tags, and validated,
// before the handler runs. O is what the route answers with, sent as JSON
// as it is. An O with Huma's own output fields (a Body field, a field with
// a header tag, a Status field) is passed to Huma untouched, for a route
// that sets headers or redirects.
func Post[I, O any](g *Group, path string, handler func(context.Context, *I) (*O, error), opts ...OperationOption) {
	register(g, http.MethodPost, path, handler, opts)
}

// Put registers a route that replaces.
func Put[I, O any](g *Group, path string, handler func(context.Context, *I) (*O, error), opts ...OperationOption) {
	register(g, http.MethodPut, path, handler, opts)
}

// Patch registers a route that changes some fields. See [Optional] for
// telling a field that was not sent from one sent as null.
func Patch[I, O any](g *Group, path string, handler func(context.Context, *I) (*O, error), opts ...OperationOption) {
	register(g, http.MethodPatch, path, handler, opts)
}

// Delete registers a route that removes.
func Delete[I, O any](g *Group, path string, handler func(context.Context, *I) (*O, error), opts ...OperationOption) {
	register(g, http.MethodDelete, path, handler, opts)
}

// Document lists in the document a route that is served by a plain Gin
// handler on the group's Router: a stream, a websocket. It registers
// nothing; the description is all the document will know of the route.
func Document(g *Group, method, path string, opts ...OperationOption) {
	op := newOperation(g, method, path, nil, opts)
	op.huma.Responses = map[string]*huma.Response{strconv.Itoa(http.StatusOK): {Description: http.StatusText(http.StatusOK)}}
	g.api.huma.OpenAPI().AddOperation(&op.huma)
}

// response is how a resource is handed to Huma: as the body.
type response[O any] struct {
	Body *O
}

func register[I, O any](g *Group, method, path string, handler func(context.Context, *I) (*O, error), opts []OperationOption) {
	outType := reflect.TypeFor[O]()

	switch {
	case outType == reflect.TypeFor[Empty]():
		op := newOperation(g, method, path, &struct{}{}, opts)
		if op.huma.DefaultStatus == 0 {
			op.huma.DefaultStatus = http.StatusNoContent
		}
		huma.Register(g.api.huma, op.huma, func(ctx context.Context, in *I) (*struct{}, error) {
			if _, err := handler(ctx, in); err != nil {
				return nil, problemFrom(ctx, err)
			}
			return nil, nil
		})
		op.document(g)

	case isHumaOutput(outType):
		op := newOperation(g, method, path, new(O), opts)
		huma.Register(g.api.huma, op.huma, func(ctx context.Context, in *I) (*O, error) {
			out, err := handler(ctx, in)
			if err != nil {
				return nil, problemFrom(ctx, err)
			}
			return out, nil
		})
		op.document(g)

	default:
		op := newOperation(g, method, path, &response[O]{}, opts)
		huma.Register(g.api.huma, op.huma, func(ctx context.Context, in *I) (*response[O], error) {
			out, err := handler(ctx, in)
			if err != nil {
				return nil, problemFrom(ctx, err)
			}
			if out == nil {
				return nil, nil
			}
			return &response[O]{Body: out}, nil
		})
		op.document(g)
	}
}

// isHumaOutput reports whether t is written the way Huma expects an output:
// a struct that names its body, a status or headers itself.
func isHumaOutput(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}
	for field := range t.Fields() {
		if field.Name == "Body" || field.Name == "Status" || field.Tag.Get("header") != "" {
			return true
		}
	}
	return false
}

// newOperation describes the route for Huma. output is a value of the type
// Huma will see, which its operation ID is derived from.
func newOperation(g *Group, method, path string, output any, opts []OperationOption) *operation {
	fullPath := g.router.BasePath() + path
	if output == nil {
		output = &struct{}{}
	}

	op := &operation{huma: huma.Operation{
		Method:      method,
		Path:        fullPath,
		OperationID: huma.GenerateOperationID(method, fullPath, output),
		Summary:     huma.GenerateSummary(method, fullPath, output),
		Tags:        slices.Clone(g.tags),
		Hidden:      g.hidden,
	}}
	for _, opt := range opts {
		opt(op)
	}

	// Every route can answer these two: its input may be invalid, and
	// anything may fail.
	statuses := []int{http.StatusBadRequest, http.StatusInternalServerError}
	for _, err := range op.errors {
		statuses = append(statuses, httperr.Status(err.Kind()))
	}
	slices.Sort(statuses)
	op.huma.Errors = slices.Compact(statuses)

	for _, scheme := range g.security {
		op.huma.Security = append(op.huma.Security, map[string][]string{scheme: append([]string{}, op.scopes...)})
	}

	op.huma.Metadata = map[string]any{routeKey: route{router: g.router, path: path, guards: op.guards}}
	return op
}

// document corrects what Huma wrote about the route's errors: validation
// answers 400 here, not 422, and each status names the codes behind it.
func (o *operation) document(g *Group) {
	item := g.api.huma.OpenAPI().Paths[o.huma.Path]
	if item == nil {
		return // hidden
	}

	var registered *huma.Operation
	switch o.huma.Method {
	case http.MethodGet:
		registered = item.Get
	case http.MethodPost:
		registered = item.Post
	case http.MethodPut:
		registered = item.Put
	case http.MethodPatch:
		registered = item.Patch
	case http.MethodDelete:
		registered = item.Delete
	}
	if registered == nil {
		return
	}

	codes := map[int][]string{
		http.StatusBadRequest:          {"request.validation_failed", "request.malformed_body"},
		http.StatusInternalServerError: {apperr.ErrInternal.Code()},
	}
	for _, err := range o.errors {
		status := httperr.Status(err.Kind())
		codes[status] = append(codes[status], err.Code())
	}

	// Huma lists a 422 for its own validation, which answers 400 here. A 422
	// stays only when one of the route's errors is one.
	if _, declared := codes[http.StatusUnprocessableEntity]; !declared {
		delete(registered.Responses, strconv.Itoa(http.StatusUnprocessableEntity))
	}
	for status, list := range codes {
		if response := registered.Responses[strconv.Itoa(status)]; response != nil {
			slices.Sort(list)
			response.Description = http.StatusText(status) + ": " + strings.Join(slices.Compact(list), ", ")
		}
	}
}

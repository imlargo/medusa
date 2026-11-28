package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/api"
	"github.com/imlargo/medusa/apperr"
	"github.com/imlargo/medusa/httperr"
	"github.com/imlargo/medusa/middleware"
)

func init() { gin.SetMode(gin.TestMode) }

var (
	errOrderNotFound = apperr.NotFound("order.not_found", "order not found")
	errOrderShipped  = apperr.FailedPrecondition("order.already_shipped", "the order has already shipped")
)

type order struct {
	ID    int64  `json:"id"`
	Item  string `json:"item"`
	Notes string `json:"notes,omitempty"`
}

type getOrderInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type createOrderInput struct {
	Body struct {
		Item     string `json:"item" minLength:"1"`
		Quantity int    `json:"quantity" minimum:"1"`
	}
}

// newAPI builds an engine with the request ID middleware, an API on it and
// a /v1 group.
func newAPI(t *testing.T, opts ...api.Option) (*gin.Engine, *api.API, *api.Group) {
	t.Helper()
	engine := gin.New()
	engine.Use(middleware.RequestID())
	docs := api.New(engine, "Orders API", "1.0.0", opts...)
	return engine, docs, docs.Group(engine.Group("/v1"), api.Tags("orders"))
}

// do sends one request and decodes a JSON answer.
func do(t *testing.T, engine *gin.Engine, method, path, body string, header ...string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	decoded := map[string]any{}
	if rec.Body.Len() > 0 && strings.Contains(rec.Header().Get("Content-Type"), "json") {
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("%s %s: body is not a JSON object: %s", method, path, rec.Body)
		}
	}
	return rec, decoded
}

// The resource is the answer: no envelope around it, nothing added to it.
func TestARouteAnswersWithTheResourceItself(t *testing.T) {
	engine, _, v1 := newAPI(t)
	api.Get(v1, "/orders/{id}", func(_ context.Context, in *getOrderInput) (*order, error) {
		return &order{ID: in.ID, Item: "lamp"}, nil
	})

	rec, body := do(t, engine, http.MethodGet, "/v1/orders/7", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if got := rec.Body.String(); strings.TrimSpace(got) != `{"id":7,"item":"lamp"}` {
		t.Errorf("body = %s, want exactly the resource", got)
	}
	if _, enveloped := body["data"]; enveloped {
		t.Error("the resource is wrapped in an envelope")
	}
}

func TestAHandlersErrorIsItsProblem(t *testing.T) {
	engine, _, v1 := newAPI(t)
	cause := errors.New("no rows in result set")
	api.Get(v1, "/orders/{id}", func(context.Context, *getOrderInput) (*order, error) {
		return nil, errOrderNotFound.WithCause(cause)
	}, api.Errors(errOrderNotFound))

	rec, body := do(t, engine, http.MethodGet, "/v1/orders/7", "")

	if rec.Code != http.StatusNotFound || rec.Header().Get("Content-Type") != httperr.ContentType {
		t.Fatalf("answer = %d %s, want 404 %s", rec.Code, rec.Header().Get("Content-Type"), httperr.ContentType)
	}
	if body["code"] != "order.not_found" || body["detail"] != "order not found" || body["request_id"] == "" {
		t.Errorf("problem = %v, want the code, the message and the request ID", body)
	}
	if strings.Contains(rec.Body.String(), "no rows") {
		t.Errorf("problem = %s, want the cause kept out of it", rec.Body)
	}
}

// An error nobody classified answers 500 and is recorded on the Gin context,
// where the access log and middleware.ReportErrors find it.
func TestAnUnclassifiedErrorIsInternalAndIsReported(t *testing.T) {
	engine := gin.New()
	var reported []error
	engine.Use(middleware.ReportErrors(func(_ *medusa.Context, err error) { reported = append(reported, err) }))
	docs := api.New(engine, "Orders API", "1.0.0")
	v1 := docs.Group(engine.Group("/v1"))

	boom := errors.New("connection reset")
	api.Get(v1, "/orders/{id}", func(context.Context, *getOrderInput) (*order, error) { return nil, boom })

	rec, body := do(t, engine, http.MethodGet, "/v1/orders/7", "")

	if rec.Code != http.StatusInternalServerError || body["code"] != apperr.ErrInternal.Code() {
		t.Errorf("answer = %d %v, want 500 %s", rec.Code, body, apperr.ErrInternal.Code())
	}
	if len(reported) != 1 || !errors.Is(reported[0], boom) {
		t.Errorf("reported = %v, want the error behind the 500", reported)
	}
}

// Validation runs before the handler, and names fields the way the client
// sent them, in Medusa's own validation error.
func TestInvalidInputIsRefusedBeforeTheHandler(t *testing.T) {
	engine, _, v1 := newAPI(t)
	called := false
	api.Post(v1, "/orders", func(context.Context, *createOrderInput) (*order, error) {
		called = true
		return &order{}, nil
	})

	rec, body := do(t, engine, http.MethodPost, "/v1/orders", `{"item":"","quantity":0}`)

	if called {
		t.Error("the handler ran on invalid input")
	}
	if rec.Code != http.StatusBadRequest || body["code"] != medusa.ErrValidation.Code() {
		t.Fatalf("answer = %d %v, want 400 %s", rec.Code, body, medusa.ErrValidation.Code())
	}
	fields := map[string]bool{}
	for _, entry := range body["errors"].([]any) {
		fields[entry.(map[string]any)["field"].(string)] = true
	}
	if !fields["item"] || !fields["quantity"] {
		t.Errorf("errors = %v, want item and quantity named without their location", body["errors"])
	}

	rec, body = do(t, engine, http.MethodGet, "/v1/orders/0", "")
	_ = rec
	if body["code"] != nil && body["code"] != medusa.ErrValidation.Code() {
		t.Errorf("an unknown route answered %v", body)
	}
}

// A field the type does not have is a client mistake, usually a typo that
// would otherwise be ignored in silence.
func TestAnUnknownBodyFieldIsRefused(t *testing.T) {
	engine, _, v1 := newAPI(t)
	api.Post(v1, "/orders", func(context.Context, *createOrderInput) (*order, error) { return &order{}, nil })

	rec, body := do(t, engine, http.MethodPost, "/v1/orders", `{"item":"lamp","quantity":1,"quantty":2}`)

	if rec.Code != http.StatusBadRequest || body["code"] != medusa.ErrValidation.Code() {
		t.Errorf("answer = %d %v, want 400 %s for an unknown field", rec.Code, body, medusa.ErrValidation.Code())
	}
}

func TestMalformedJSONIsAMalformedBody(t *testing.T) {
	engine, _, v1 := newAPI(t)
	api.Post(v1, "/orders", func(context.Context, *createOrderInput) (*order, error) { return &order{}, nil })

	rec, body := do(t, engine, http.MethodPost, "/v1/orders", `{"item":`)

	if rec.Code != http.StatusBadRequest || body["code"] != medusa.ErrMalformedBody.Code() {
		t.Errorf("answer = %d %v, want 400 %s", rec.Code, body, medusa.ErrMalformedBody.Code())
	}
}

func TestStatusesOtherThan200(t *testing.T) {
	engine, _, v1 := newAPI(t)
	api.Post(v1, "/orders", func(context.Context, *createOrderInput) (*order, error) {
		return &order{ID: 1}, nil
	}, api.Status(http.StatusCreated))
	api.Post(v1, "/exports", func(context.Context, *struct{}) (*order, error) {
		return &order{ID: 2}, nil
	}, api.Status(http.StatusAccepted))
	api.Delete(v1, "/orders/{id}", func(context.Context, *getOrderInput) (*api.Empty, error) { return nil, nil })

	if rec, _ := do(t, engine, http.MethodPost, "/v1/orders", `{"item":"lamp","quantity":1}`); rec.Code != http.StatusCreated {
		t.Errorf("create = %d, want 201", rec.Code)
	}
	if rec, _ := do(t, engine, http.MethodPost, "/v1/exports", ""); rec.Code != http.StatusAccepted {
		t.Errorf("accept = %d, want 202", rec.Code)
	}
	rec, _ := do(t, engine, http.MethodDelete, "/v1/orders/7", "")
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Errorf("delete = %d %q, want 204 with no body", rec.Code, rec.Body)
	}
}

// A route that redirects or sets headers writes its output the way Huma
// reads it, and is passed through.
func TestAnOutputWithHeadersIsPassedToHuma(t *testing.T) {
	type download struct {
		Status       int
		Location     string `header:"Location"`
		CacheControl string `header:"Cache-Control"`
	}

	engine, _, v1 := newAPI(t)
	api.Get(v1, "/orders/{id}/invoice", func(context.Context, *getOrderInput) (*download, error) {
		return &download{Status: http.StatusFound, Location: "https://files.example.com/signed", CacheControl: "no-store"}, nil
	})

	rec, _ := do(t, engine, http.MethodGet, "/v1/orders/7/invoice", "")

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://files.example.com/signed" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("answer = %d, Location %q, Cache-Control %q, want a 302 with both headers", rec.Code, rec.Header().Get("Location"), rec.Header().Get("Cache-Control"))
	}
}

// A partial update has to tell "not sent" from "sent as null" from a value.
func TestOptionalTellsAbsentFromNullFromAValue(t *testing.T) {
	type updateInput struct {
		ID   int64 `path:"id"`
		Body struct {
			Item  api.Optional[string] `json:"item,omitzero"`
			Notes api.Optional[string] `json:"notes,omitzero"`
			Count api.Optional[int]    `json:"count,omitzero"`
		}
	}

	var got updateInput
	engine, _, v1 := newAPI(t)
	api.Patch(v1, "/orders/{id}", func(_ context.Context, in *updateInput) (*api.Empty, error) {
		got = *in
		return nil, nil
	})

	rec, _ := do(t, engine, http.MethodPatch, "/v1/orders/7", `{"item":"desk","notes":null}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}

	if value, ok := got.Body.Item.Get(); !ok || value != "desk" {
		t.Errorf("item = %+v, want the value desk", got.Body.Item)
	}
	if !got.Body.Notes.Sent || !got.Body.Notes.Null {
		t.Errorf("notes = %+v, want sent as null", got.Body.Notes)
	}
	if got.Body.Count.Sent {
		t.Errorf("count = %+v, want not sent", got.Body.Count)
	}

	// The value is still validated as its type.
	if rec, _ := do(t, engine, http.MethodPatch, "/v1/orders/7", `{"count":"many"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a string for an integer field = %d, want 400", rec.Code)
	}
}

// A webhook receiver needs the exact bytes that were signed, and every
// header, without the body being parsed.
func TestARawBodyRouteGetsTheExactBytesAndTheRequest(t *testing.T) {
	type webhookInput struct {
		RawBody []byte
	}

	var gotBody, gotHeader string
	engine, _, v1 := newAPI(t)
	api.Post(v1, "/hooks/billing", func(ctx context.Context, in *webhookInput) (*api.Empty, error) {
		gotBody = string(in.RawBody)
		gotHeader = api.Request(ctx).Header.Get("webhook-signature")
		return nil, nil
	}, api.Hidden())

	const sent = `{ "id":"evt_1",   "spacing": "kept" }`
	rec, _ := do(t, engine, http.MethodPost, "/v1/hooks/billing", sent, "webhook-signature", "v1,abc")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if gotBody != sent || gotHeader != "v1,abc" {
		t.Errorf("handler got body %q and signature %q, want the bytes as sent and the header", gotBody, gotHeader)
	}
}

type listInput struct {
	api.Page
}

func TestListsPageWithOneCursorShape(t *testing.T) {
	all := []order{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}}

	engine, _, v1 := newAPI(t)
	api.Get(v1, "/orders", func(_ context.Context, in *listInput) (*api.List[order], error) {
		var after int64
		if _, err := api.DecodeCursor(in.Cursor, &after); err != nil {
			return nil, err
		}

		var rows []order
		for _, o := range all {
			if o.ID > after && len(rows) < in.Limit+1 { // one more than the page
				rows = append(rows, o)
			}
		}
		return api.NewList(rows, in.Limit, func(last order) any { return last.ID }), nil
	})

	page := func(query string) (int, []any, string) {
		rec, body := do(t, engine, http.MethodGet, "/v1/orders"+query, "")
		items, _ := body["items"].([]any)
		cursor, _ := body["next_cursor"].(string)
		return rec.Code, items, cursor
	}

	status, items, cursor := page("?limit=2")
	if status != http.StatusOK || len(items) != 2 || cursor == "" {
		t.Fatalf("first page = %d, %d items, cursor %q, want 2 items and a cursor", status, len(items), cursor)
	}
	_, items, cursor = page("?limit=2&cursor=" + cursor)
	if len(items) != 2 || items[0].(map[string]any)["id"] != float64(3) || cursor == "" {
		t.Errorf("second page = %v, cursor %q, want orders 3 and 4 and a cursor", items, cursor)
	}
	_, items, cursor = page("?limit=2&cursor=" + cursor)
	if len(items) != 1 || cursor != "" {
		t.Errorf("last page = %v, cursor %q, want order 5 and no cursor", items, cursor)
	}

	// The default and the maximum come from api.Page, for every list.
	if _, items, _ := page(""); len(items) != 5 {
		t.Errorf("without a limit = %d items, want all 5 under the default of %d", len(items), api.DefaultLimit)
	}
	if status, _, _ := page("?limit=101"); status != http.StatusBadRequest {
		t.Errorf("limit over the maximum = %d, want 400", status)
	}

	rec, body := do(t, engine, http.MethodGet, "/v1/orders?cursor=not-a-cursor", "")
	if rec.Code != http.StatusBadRequest || body["code"] != api.ErrInvalidCursor.Code() {
		t.Errorf("a made-up cursor = %d %v, want 400 %s", rec.Code, body, api.ErrInvalidCursor.Code())
	}
}

// An empty list is "items": [], never null: clients iterate it either way.
func TestAnEmptyListIsAnEmptyArray(t *testing.T) {
	encoded, err := json.Marshal(api.NewList[order](nil, 20, func(order) any { return nil }))
	if err != nil || string(encoded) != `{"items":[]}` {
		t.Errorf("empty list = %s, %v, want {\"items\":[]}", encoded, err)
	}
}

type principalKey struct{}

// The guard and its documentation are one declaration: the middleware that
// protects the group is given where the scheme is named, and the scopes
// where the route is registered.
func TestSecurityIsDeclaredOnceAndEnforced(t *testing.T) {
	// Stand-ins for auth.Middleware and auth.Require.
	authenticate := func(c *gin.Context) {
		scopes := c.GetHeader("X-Scopes")
		if c.GetHeader("Authorization") == "" {
			medusa.AbortWithError(c, medusa.ErrMissingCredentials)
			return
		}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), principalKey{}, scopes))
		c.Next()
	}
	denied := apperr.PermissionDenied("auth.insufficient_scope", "not allowed")
	require := func(scopes ...string) gin.HandlerFunc {
		return func(c *gin.Context) {
			granted, _ := c.Request.Context().Value(principalKey{}).(string)
			for _, scope := range scopes {
				if !strings.Contains(granted, scope) {
					medusa.AbortWithError(c, denied)
					return
				}
			}
			c.Next()
		}
	}

	engine := gin.New()
	docs := api.New(engine, "Orders API", "1.0.0", api.WithBearerAuth())
	v1 := docs.Group(engine.Group("/v1"), api.Secured(api.BearerAuth, authenticate))

	var seenByHandler string
	api.Post(v1, "/orders", func(ctx context.Context, _ *createOrderInput) (*order, error) {
		// What middleware put in the request's context reaches the handler.
		seenByHandler, _ = ctx.Value(principalKey{}).(string)
		return &order{}, nil
	}, api.Scopes(require, "orders:write"))

	const body = `{"item":"lamp","quantity":1}`
	if rec, _ := do(t, engine, http.MethodPost, "/v1/orders", body); rec.Code != http.StatusUnauthorized {
		t.Errorf("without credentials = %d, want 401", rec.Code)
	}
	if rec, _ := do(t, engine, http.MethodPost, "/v1/orders", body, "Authorization", "Bearer x", "X-Scopes", "orders:read"); rec.Code != http.StatusForbidden {
		t.Errorf("without the scope = %d, want 403", rec.Code)
	}
	if rec, _ := do(t, engine, http.MethodPost, "/v1/orders", body, "Authorization", "Bearer x", "X-Scopes", "orders:write"); rec.Code != http.StatusOK {
		t.Errorf("with the scope = %d, want 200", rec.Code)
	}
	if seenByHandler != "orders:write" {
		t.Errorf("the handler's context carried %q, want what the middleware put in the request's context", seenByHandler)
	}

	security := docs.OpenAPI().Paths["/v1/orders"].Post.Security
	if len(security) != 1 || len(security[0][api.BearerAuth]) != 1 || security[0][api.BearerAuth][0] != "orders:write" {
		t.Errorf("documented security = %v, want bearerAuth with orders:write", security)
	}
}

func TestTheDocumentDescribesWhatTheRoutesDo(t *testing.T) {
	engine, docs, v1 := newAPI(t, api.WithDocs(true))
	api.Get(v1, "/orders/{id}", func(context.Context, *getOrderInput) (*order, error) { return &order{}, nil },
		api.Summary("Get an order"), api.Errors(errOrderNotFound))
	api.Post(v1, "/orders/{id}/ship", func(context.Context, *getOrderInput) (*order, error) { return &order{}, nil },
		api.Errors(errOrderNotFound, errOrderShipped), api.Idempotent())
	api.Get(v1, "/internal", func(context.Context, *struct{}) (*order, error) { return &order{}, nil }, api.Hidden())
	api.Document(v1, http.MethodGet, "/events", api.Summary("Stream of events"), api.Description("Server-sent events."))
	v1.Router().GET("/events", func(c *gin.Context) { c.Status(http.StatusOK) })

	paths := docs.OpenAPI().Paths

	get := paths["/v1/orders/{id}"].Get
	if get.Summary != "Get an order" || len(get.Tags) != 1 || get.Tags[0] != "orders" {
		t.Errorf("get = summary %q, tags %v, want the summary given and the group's tag", get.Summary, get.Tags)
	}
	if got := get.Responses["404"]; got == nil || !strings.Contains(got.Description, "order.not_found") {
		t.Errorf("404 = %+v, want it to name order.not_found", got)
	}
	if got := get.Responses["400"]; got == nil || !strings.Contains(got.Description, "request.validation_failed") {
		t.Errorf("400 = %+v, want it to name request.validation_failed", got)
	}
	if _, has := get.Responses["422"]; has {
		t.Error("the document lists a 422, which no route answers")
	}

	ship := paths["/v1/orders/{id}/ship"].Post
	if got := ship.Responses["422"]; got == nil || !strings.Contains(got.Description, "order.already_shipped") {
		t.Errorf("422 of ship = %+v, want order.already_shipped: a failed precondition is a 422 someone declared", got)
	}
	hasKey := false
	for _, param := range ship.Parameters {
		hasKey = hasKey || (param.Name == "Idempotency-Key" && param.In == "header")
	}
	if !hasKey {
		t.Error("ship does not document the Idempotency-Key header")
	}

	if _, listed := paths["/v1/internal"]; listed {
		t.Error("a hidden route is in the document")
	}
	if events := paths["/v1/events"]; events == nil || events.Get == nil || events.Get.Summary != "Stream of events" {
		t.Errorf("the stream documented with Document is not in the document: %+v", events)
	}

	// The document and its page are served only because WithDocs is on.
	if rec, _ := do(t, engine, http.MethodGet, "/openapi.json", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"3.1`) {
		t.Errorf("GET /openapi.json = %d, want the OpenAPI 3.1 document", rec.Code)
	}
	if rec, _ := do(t, engine, http.MethodGet, "/docs", ""); rec.Code != http.StatusOK {
		t.Errorf("GET /docs = %d, want the documentation page", rec.Code)
	}
}

func TestTheDocumentIsNotServedByDefault(t *testing.T) {
	engine, _, _ := newAPI(t)

	for _, path := range []string{"/openapi.json", "/docs"} {
		if rec, _ := do(t, engine, http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 without WithDocs", path, rec.Code)
		}
	}
}

// Typed routes and plain Gin routes share a group and its middleware.
func TestTypedAndPlainRoutesShareAGroup(t *testing.T) {
	engine := gin.New()
	docs := api.New(engine, "Orders API", "1.0.0")

	group := engine.Group("/v1", func(c *gin.Context) {
		c.Header("X-Guarded", "yes")
		c.Next()
	})
	v1 := docs.Group(group)

	api.Get(v1, "/typed", func(context.Context, *struct{}) (*order, error) { return &order{}, nil })
	v1.Router().GET("/plain", medusa.HandleGet(func(*medusa.Context) (*order, error) { return &order{}, nil }))

	for _, path := range []string{"/v1/typed", "/v1/plain"} {
		if rec, _ := do(t, engine, http.MethodGet, path, ""); rec.Code != http.StatusOK || rec.Header().Get("X-Guarded") != "yes" {
			t.Errorf("GET %s = %d, guarded %q, want 200 behind the group's middleware", path, rec.Code, rec.Header().Get("X-Guarded"))
		}
	}
}

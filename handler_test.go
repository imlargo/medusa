package medusa_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/apperr"
)

var (
	errUserNotFound = apperr.NotFound("user.not_found", "user not found")
	errEmailTaken   = apperr.Conflict("user.email_taken", "a user with this email already exists")
)

func init() { gin.SetMode(gin.TestMode) }

type greeting struct {
	Name string `json:"name" binding:"required"`
}

type greeted struct {
	Message string `json:"message"`
}

// call routes one request through handler and returns the status and body.
func call(t *testing.T, method, path, body string, handler gin.HandlerFunc) (int, map[string]any) {
	t.Helper()

	router := gin.New()
	router.Handle(method, "/x", handler)

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	decoded := map[string]any{}
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("body is not JSON: %v (%q)", err, rec.Body.String())
		}
	}
	return rec.Code, decoded
}

func TestHandleRepliesOK(t *testing.T) {
	status, body := call(t, http.MethodPost, "/x", `{"name":"ada"}`,
		medusa.Handle(func(ctx *medusa.Context, in *greeting) (*greeted, error) {
			return &greeted{Message: "hello " + in.Name}, nil
		}))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if body["success"] != true {
		t.Errorf("success = %v, want true", body["success"])
	}

	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %v, want an object", body["data"])
	}
	if data["message"] != "hello ada" {
		t.Errorf("message = %v, want %q", data["message"], "hello ada")
	}
}

func TestHandleCreateReplies201(t *testing.T) {
	status, _ := call(t, http.MethodPost, "/x", `{"name":"ada"}`,
		medusa.HandleCreate(func(ctx *medusa.Context, in *greeting) (*greeted, error) {
			return &greeted{Message: "created"}, nil
		}))

	if status != http.StatusCreated {
		t.Errorf("status = %d, want %d", status, http.StatusCreated)
	}
}

// A missing required field must never reach the handler.
func TestHandleRejectsAnInvalidBodyBeforeTheHandlerRuns(t *testing.T) {
	called := false

	status, body := call(t, http.MethodPost, "/x", `{}`,
		medusa.Handle(func(ctx *medusa.Context, in *greeting) (*greeted, error) {
			called = true
			return nil, nil
		}))

	if called {
		t.Error("handler ran, want validation to reject the request first")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", status, http.StatusBadRequest)
	}
	if body["code"] != medusa.ErrValidation.Code() {
		t.Errorf("code = %v, want %q", body["code"], medusa.ErrValidation.Code())
	}

	fields, ok := body["errors"].([]any)
	if !ok || len(fields) != 1 {
		t.Fatalf("errors = %v, want one field error", body["errors"])
	}
	field, _ := fields[0].(map[string]any)
	if field["field"] != "name" || field["message"] != "this field is required" {
		t.Errorf("errors[0] = %v, want name: this field is required", field)
	}
}

// This is the payoff of returning errors: the handler says what went wrong and
// the status follows from that, with no response code at the call site.
func TestHandleDerivesTheStatusFromTheReturnedError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"not found", errUserNotFound, http.StatusNotFound},
		{"conflict", errEmailTaken, http.StatusConflict},
		{"unauthenticated", medusa.ErrMissingCredentials, http.StatusUnauthorized},
		{"unclassified becomes 500", errors.New("boom"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := call(t, http.MethodPost, "/x", `{"name":"ada"}`,
				medusa.Handle(func(ctx *medusa.Context, in *greeting) (*greeted, error) {
					return nil, tt.err
				}))

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
		})
	}
}

func TestHandleGetNeedsNoBody(t *testing.T) {
	status, body := call(t, http.MethodGet, "/x", "",
		medusa.HandleGet(func(ctx *medusa.Context) (*greeted, error) {
			return &greeted{Message: "ok"}, nil
		}))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if body["success"] != true {
		t.Errorf("success = %v, want true", body["success"])
	}
}

func TestHandleDelete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		status, body := call(t, http.MethodDelete, "/x", "",
			medusa.HandleDelete(func(ctx *medusa.Context) error { return nil }))

		if status != http.StatusOK {
			t.Fatalf("status = %d, want %d", status, http.StatusOK)
		}
		if body["message"] != medusa.MessageDeleted {
			t.Errorf("message = %v, want %q", body["message"], medusa.MessageDeleted)
		}
	})

	t.Run("failure", func(t *testing.T) {
		status, _ := call(t, http.MethodDelete, "/x", "",
			medusa.HandleDelete(func(ctx *medusa.Context) error {
				return errUserNotFound
			}))

		if status != http.StatusNotFound {
			t.Errorf("status = %d, want %d", status, http.StatusNotFound)
		}
	})
}

// Handler is for endpoints that write their own response; on success nothing
// should be appended to what the handler already sent.
func TestHandlerLeavesASuccessfulResponseAlone(t *testing.T) {
	status, body := call(t, http.MethodGet, "/x", "",
		medusa.Handler(func(ctx *medusa.Context) error {
			ctx.JSON(http.StatusTeapot, map[string]string{"custom": "yes"})
			return nil
		}))

	if status != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", status, http.StatusTeapot)
	}
	if body["custom"] != "yes" {
		t.Errorf("body = %v, want the handler's own payload", body)
	}
}

type order struct {
	Items []orderItem `json:"items" binding:"required,dive"`
}

type orderItem struct {
	UnitPrice int `json:"unit_price" binding:"min=1"`
}

// Clients know their fields by the names they sent, not by the Go names, and
// a field inside a list needs its index to be found.
func TestValidationErrorsNameFieldsAsTheClientSentThem(t *testing.T) {
	_, body := call(t, http.MethodPost, "/x", `{"items":[{"unit_price":5},{"unit_price":0}]}`,
		medusa.Handle(func(ctx *medusa.Context, in *order) (*greeted, error) {
			return &greeted{}, nil
		}))

	fields, _ := body["errors"].([]any)
	if len(fields) != 1 {
		t.Fatalf("errors = %v, want one field error", body["errors"])
	}
	field, _ := fields[0].(map[string]any)
	if field["field"] != "items[1].unit_price" {
		t.Errorf("field = %v, want items[1].unit_price", field["field"])
	}
}

func TestPaginationFieldsAreNamedByTheirQueryParameter(t *testing.T) {
	_, body := call(t, http.MethodGet, "/x?page_size=-1", "", func(c *gin.Context) {
		if _, err := medusa.NewContext(c).Pagination(); err != nil {
			medusa.WriteError(c, err)
		}
	})

	fields, _ := body["errors"].([]any)
	if len(fields) != 1 {
		t.Fatalf("errors = %v, want one field error", body["errors"])
	}
	if field, _ := fields[0].(map[string]any); field["field"] != "page_size" {
		t.Errorf("field = %v, want page_size", field["field"])
	}
}

// A query parameter that does not parse is a bad parameter, not a bad body.
func TestUnparsableQueryIsAnInvalidParameter(t *testing.T) {
	status, body := call(t, http.MethodGet, "/x?page_size=abc", "", func(c *gin.Context) {
		if _, err := medusa.NewContext(c).Pagination(); err != nil {
			medusa.WriteError(c, err)
		}
	})

	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
	if body["code"] != medusa.ErrInvalidParameter.Code() {
		t.Errorf("code = %v, want %q", body["code"], medusa.ErrInvalidParameter.Code())
	}
}

// sort_by usually lands in an ORDER BY, so a field nobody allowed must never
// pass through, even when the caller forgot the allow list.
func TestSortAcceptsOnlyAllowedFields(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		allowed []string
		want    string
	}{
		{"allowed field", "sort_by=name", []string{"name", "created_at"}, "name"},
		{"unknown field", "sort_by=password", []string{"name", "created_at"}, "created_at"},
		{"no allow list", "sort_by=password;drop", nil, "created_at"},
		{"default not allowed", "", []string{"name"}, "name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got medusa.SortParams
			call(t, http.MethodGet, "/x?"+tt.query, "", func(c *gin.Context) {
				got = medusa.NewContext(c).Sort("created_at", medusa.SortOrderDesc, tt.allowed...)
			})
			if got.Field != tt.want {
				t.Errorf("Sort().Field = %q, want %q", got.Field, tt.want)
			}
		})
	}
}

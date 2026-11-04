package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/imlargo/medusa/postgres/pgtest"

	"github.com/imlargo/medusa/app/internal/account"
	"github.com/imlargo/medusa/app/internal/database/migrations"
	"github.com/imlargo/medusa/app/internal/database/sqlc"
	"github.com/imlargo/medusa/app/internal/user"
)

// The account flow end to end, through the real router and middleware and a
// real database: register, sign in, read the signed-in user, and the error
// contract on the way.
func TestAccountFlow(t *testing.T) {
	pool := pgtest.New(t, migrations.FS)

	router := newRouter(newTestContainer(t, func(c *Container) {
		service := account.NewService(user.NewRepository(sqlc.New(pool)), c.JWT, time.Minute, time.Hour)
		c.Handlers.Account = account.NewHandler(service)
	}))

	send := func(method, path, body, token string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		decoded := map[string]any{}
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("%s %s: body is not JSON: %s", method, path, rec.Body)
		}
		return rec.Code, decoded
	}
	data := func(body map[string]any) map[string]any {
		d, _ := body["data"].(map[string]any)
		return d
	}

	status, body := send(http.MethodPost, "/v1/auth/register", `{"email":"Ada@Example.com","password":"correct horse"}`, "")
	if status != http.StatusCreated {
		t.Fatalf("register = %d %v, want 201", status, body)
	}

	status, body = send(http.MethodPost, "/v1/auth/register", `{"email":"ada@example.com","password":"another one"}`, "")
	if status != http.StatusConflict || body["code"] != user.ErrEmailTaken.Code() {
		t.Errorf("register again = %d %v, want 409 %s", status, body, user.ErrEmailTaken.Code())
	}

	status, body = send(http.MethodPost, "/v1/auth/login", `{"email":"ada@example.com","password":"wrong"}`, "")
	if status != http.StatusUnauthorized || body["code"] != account.ErrInvalidCredentials.Code() {
		t.Errorf("login with a wrong password = %d %v, want 401 %s", status, body, account.ErrInvalidCredentials.Code())
	}

	status, body = send(http.MethodPost, "/v1/auth/login", `{"email":"ADA@example.com","password":"correct horse"}`, "")
	if status != http.StatusOK {
		t.Fatalf("login = %d %v, want 200", status, body)
	}
	tokens, _ := data(body)["tokens"].(map[string]any)
	access, _ := tokens["access_token"].(string)
	if access == "" {
		t.Fatalf("login tokens = %v, want an access token", tokens)
	}

	status, body = send(http.MethodGet, "/v1/auth/user", "", access)
	if status != http.StatusOK || data(body)["email"] != "ada@example.com" {
		t.Errorf("me = %d %v, want 200 for ada@example.com", status, body)
	}
}

package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/postgres/pgtest"
	"github.com/imlargo/medusa/postgres/queue"

	"github.com/imlargo/medusa/app/internal/account"
	"github.com/imlargo/medusa/app/internal/database/migrations"
	"github.com/imlargo/medusa/app/internal/store"
	"github.com/imlargo/medusa/app/internal/user"
)

// The account flow end to end, through the real router and middleware and a
// real database: register, sign in, read the signed-in user, and the error
// contract on the way.
func TestAccountFlow(t *testing.T) {
	pool := pgtest.New(t, migrations.FS)
	if err := queue.Install(t.Context(), pool); err != nil {
		t.Fatalf("install the job queue: %v", err)
	}

	router := newRouter(newTestContainer(t, func(c *Container) {
		// No reuse grace: a second use of a refresh token is a stolen one.
		service := account.NewService(store.New(pool), c.JWT, time.Minute, time.Hour, 0)
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
	sendNoBody := func(method, path, body string) int {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
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

	// One account, one welcome job: the rejected registration rolled its own
	// back with the insert it belonged to.
	var welcomes int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM medusa_jobs WHERE kind = $1", account.KindWelcome).Scan(&welcomes); err != nil {
		t.Fatalf("count welcome jobs: %v", err)
	}
	if welcomes != 1 {
		t.Errorf("welcome jobs = %d, want 1", welcomes)
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

	// A refresh token is not an access token: it opens nothing by itself.
	refresh, _ := tokens["refresh_token"].(string)
	if status, _ = send(http.MethodGet, "/v1/auth/user", "", refresh); status != http.StatusUnauthorized {
		t.Errorf("me with the refresh token as bearer = %d, want 401", status)
	}

	status, body = send(http.MethodPost, "/v1/auth/refresh", `{"refresh_token":"`+refresh+`"}`, "")
	if status != http.StatusOK {
		t.Fatalf("refresh = %d %v, want 200", status, body)
	}
	rotated, _ := data(body)["refresh_token"].(string)
	newAccess, _ := data(body)["access_token"].(string)
	if rotated == "" || rotated == refresh {
		t.Errorf("refresh returned %q, want a new refresh token", rotated)
	}
	if status, _ = send(http.MethodGet, "/v1/auth/user", "", newAccess); status != http.StatusOK {
		t.Errorf("me with the refreshed access token = %d, want 200", status)
	}

	// The old refresh token was copied, as far as the server can tell: it is
	// refused, and the session it belonged to ends.
	status, body = send(http.MethodPost, "/v1/auth/refresh", `{"refresh_token":"`+refresh+`"}`, "")
	if status != http.StatusUnauthorized || body["code"] != auth.ErrInvalidRefreshToken.Code() {
		t.Errorf("refresh with a used token = %d %v, want 401 %s", status, body, auth.ErrInvalidRefreshToken.Code())
	}
	if status, _ = send(http.MethodPost, "/v1/auth/refresh", `{"refresh_token":"`+rotated+`"}`, ""); status != http.StatusUnauthorized {
		t.Errorf("refresh with the session's current token after a reuse = %d, want 401", status)
	}

	// Signing out ends a fresh session, and is safe to repeat.
	_, body = send(http.MethodPost, "/v1/auth/login", `{"email":"ada@example.com","password":"correct horse"}`, "")
	tokens, _ = data(body)["tokens"].(map[string]any)
	refresh, _ = tokens["refresh_token"].(string)
	for range 2 {
		if status := sendNoBody(http.MethodPost, "/v1/auth/logout", `{"refresh_token":"`+refresh+`"}`); status != http.StatusNoContent {
			t.Errorf("logout = %d, want 204", status)
		}
	}
	if status, _ = send(http.MethodPost, "/v1/auth/refresh", `{"refresh_token":"`+refresh+`"}`, ""); status != http.StatusUnauthorized {
		t.Errorf("refresh after logout = %d, want 401", status)
	}
}

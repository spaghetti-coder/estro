package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	echoMiddleware "github.com/labstack/echo/v5/middleware"
	"github.com/spaghetti-coder/estro/internal/auth"
	"github.com/spaghetti-coder/estro/internal/config"
	"github.com/spaghetti-coder/estro/internal/job"
	appMiddleware "github.com/spaghetti-coder/estro/internal/middleware"
)

func ptrOf[T any](v T) *T { return &v }

// loadConfig validates cfg and returns a LoadResult. No YAML round-trip.
func loadConfig(t *testing.T, cfg *config.Config) *config.LoadResult {
	t.Helper()
	issues := config.Validate(cfg)
	res := &config.LoadResult{Config: cfg, Issues: issues}
	if !res.Healthy() {
		t.Fatalf("invalid test config: %v", res.IssueStrings())
	}
	return res
}

func buildTestConfig() *config.Config {
	return &config.Config{
		Global: &config.GlobalConfig{
			Title:    ptrOf("Estro"),
			Hostname: ptrOf("127.0.0.1"),
			Port:     ptrOf(3000),
			CascadeFields: config.CascadeFields{
				Timeout:    ptrOf(30),
				Confirm:    ptrOf(true),
				Restricted: ptrOf(false),
			},
		},
		Users: map[string]*config.UserConfig{
			"alice": {Password: "$2a$04$RaT2a8ho6k.L3WboREnlDOzOFvVRT4BZ27qHvoYjWIy.f86sEzRCi", Groups: config.StringList{"admins"}},
			"bob":   {Password: "$2a$04$eFgD5RBrhd9knn7y65gOo.puc0zS7aU/5vE9rtR2Zoh4W.PQN2Qti", Groups: config.StringList{"admins", "family"}},
			"guest": {Password: "$2a$04$BHALOlcRQPMb435x.vD/0OTDO0fqn8e6iVb4BkIgVKuirpah7u0tW"},
		},
		Sections: []config.SectionConfig{
			{
				Title:        "Public Info",
				LayoutFields: config.LayoutFields{Collapsable: ptrOf(false), Columns: ptrOf(1)},
				Services: []config.ServiceConfig{
					{Title: "Uptime", Command: config.CommandValue{"uptime"}},
					{Title: "Date", Command: config.CommandValue{"date"}, CascadeFields: config.CascadeFields{Confirm: ptrOf(false)}},
				},
			},
			{
				Title:         "Admin",
				CascadeFields: config.CascadeFields{Allowed: config.StringList{"admins"}},
				LayoutFields:  config.LayoutFields{Columns: ptrOf(4)},
				Services: []config.ServiceConfig{
					{Title: "List /etc", Command: config.CommandValue{"ls -lah /etc"}},
					{Title: "Who", Command: config.CommandValue{"who"}, CascadeFields: config.CascadeFields{Confirm: ptrOf(false)}},
				},
			},
			{
				Title:         "Mixed Access",
				CascadeFields: config.CascadeFields{Allowed: config.StringList{"admins"}},
				Services: []config.ServiceConfig{
					{Title: "Public status", Command: config.CommandValue{"uptime"}, CascadeFields: config.CascadeFields{Allowed: config.StringList{}, Confirm: ptrOf(false)}},
					{Title: "Admin only", Command: config.CommandValue{"id"}},
					{Title: "Guest allowed", Command: config.CommandValue{"date"}, CascadeFields: config.CascadeFields{Allowed: config.StringList{"guest"}}},
				},
			},
		},
	}
}

func buildRestrictedTestConfig() *config.Config {
	return &config.Config{
		Global: &config.GlobalConfig{
			Title:    ptrOf("Estro"),
			Hostname: ptrOf("127.0.0.1"),
			Port:     ptrOf(3000),
			CascadeFields: config.CascadeFields{
				Timeout:    ptrOf(30),
				Confirm:    ptrOf(true),
				Restricted: ptrOf(false),
			},
		},
		Users: map[string]*config.UserConfig{
			"alice": {Password: "$2a$04$RaT2a8ho6k.L3WboREnlDOzOFvVRT4BZ27qHvoYjWIy.f86sEzRCi", Groups: config.StringList{"admins"}},
			"guest": {Password: "$2a$04$BHALOlcRQPMb435x.vD/0OTDO0fqn8e6iVb4BkIgVKuirpah7u0tW"},
		},
		Sections: []config.SectionConfig{
			{
				Title: "Public section",
				Services: []config.ServiceConfig{
					{Title: "Uptime", Command: config.CommandValue{"uptime"}},
				},
			},
			{
				Title:         "Admin section",
				CascadeFields: config.CascadeFields{Allowed: config.StringList{"admins"}, Restricted: ptrOf(true)},
				Services: []config.ServiceConfig{
					{Title: "Admin tool", Command: config.CommandValue{"id"}},
				},
			},
			{
				Title:         "Visible restricted section",
				CascadeFields: config.CascadeFields{Allowed: config.StringList{"admins"}, Restricted: ptrOf(false)},
				Services: []config.ServiceConfig{
					{Title: "Visible admin tool", Command: config.CommandValue{"whoami"}},
				},
			},
		},
	}
}

func setupTestEnvWithConfig(t *testing.T, cfg *config.Config) (*echo.Echo, *Handler, *job.Store) {
	t.Helper()
	res := loadConfig(t, cfg)
	store := job.NewStore()
	sessionSecret, err := auth.GenerateSessionSecret()
	if err != nil {
		t.Fatal(err)
	}
	sessionStore := auth.NewSessionStore(sessionSecret)
	h := NewHandler(res, store, sessionStore, context.Background())

	e := echo.New()
	e.Use(appMiddleware.SecurityMiddleware("default-src 'self'"))
	e.Use(appMiddleware.FaviconCORS())
	e.Use(echoMiddleware.RequestLogger())
	h.RegisterRoutes(e)
	return e, h, store
}

func setupTestEnv(t *testing.T) (*echo.Echo, *Handler, *job.Store) {
	t.Helper()
	return setupTestEnvWithConfig(t, buildTestConfig())
}

func loginAs(t *testing.T, e *echo.Echo, username, password string, rememberMe bool) *http.Cookie {
	t.Helper()
	body := `{"username":"` + username + `","password":"` + password + `"`
	if rememberMe {
		body += `,"rememberMe":true`
	}
	body += `}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	cookie := extractSessionCookie(rec)
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}
	return cookie
}

// extractSessionCookie parses the Set-Cookie header directly; works around Go's
// ReadSetCookies rejecting '|' in cookie values (gorilla securecookie uses '|' as separator).
func extractSessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, v := range rec.Header().Values("Set-Cookie") {
		if !strings.HasPrefix(v, "connect.sid=") {
			continue
		}
		parts := strings.SplitN(v, "=", 2)
		val := parts[1]
		var maxAge int
		if idx := strings.Index(val, ";"); idx >= 0 {
			// Parse attributes from the remainder
			attrs := val[idx+1:]
			val = val[:idx]
			for _, attr := range strings.Split(attrs, ";") {
				attr = strings.TrimSpace(attr)
				if strings.HasPrefix(attr, "Max-Age=") {
					_, _ = fmt.Sscanf(attr, "Max-Age=%d", &maxAge)
				}
			}
		}
		return &http.Cookie{Name: "connect.sid", Value: val, MaxAge: maxAge}
	}
	return nil
}

// hasSessionCookie checks if the response has a Set-Cookie header for connect.sid.
func hasSessionCookie(rec *httptest.ResponseRecorder) bool {
	for _, v := range rec.Header().Values("Set-Cookie") {
		if strings.HasPrefix(v, "connect.sid=") {
			return true
		}
	}
	return false
}

// decodeJSON is a test helper to decode a JSON response body.
func decodeJSON(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
}

func doRequest(e *echo.Echo, method, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func doRequestWithBody(e *echo.Echo, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func createJob(t *testing.T, e *echo.Echo, cookie *http.Cookie) string {
	t.Helper()
	rec := doRequest(e, http.MethodPost, "/run/0", cookie)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("createJob: expected 202, got %d", rec.Code)
	}
	var result map[string]any
	decodeJSON(t, rec.Body.Bytes(), &result)
	jobID, _ := result["jobId"].(string)
	if jobID == "" {
		t.Fatal("createJob: no jobId in response")
	}
	return jobID
}

func TestGetConfig(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	rec := doRequest(e, http.MethodGet, "/config", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result map[string]any
	decodeJSON(t, rec.Body.Bytes(), &result)
	if result["title"] != "Estro" {
		t.Errorf("expected title 'Estro', got %v", result["title"])
	}
	if result["subtitle"] != "" {
		t.Errorf("expected empty subtitle, got %v", result["subtitle"])
	}
	if users, ok := result["users"].([]any); !ok || len(users) != 3 {
		t.Errorf("expected 3 users, got %v", result["users"])
	}
	if result["degraded"] != false {
		t.Errorf("expected degraded=false, got %v", result["degraded"])
	}
}

func TestListServicesUnauthenticated(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	rec := doRequest(e, http.MethodGet, "/services", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result []any
	decodeJSON(t, rec.Body.Bytes(), &result)
	if len(result) == 0 {
		t.Error("expected services, got empty array")
	}
}

func TestListServicesAuthenticatedAlice(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	cookie := loginAs(t, e, "alice", "changeme1", false)
	rec := doRequest(e, http.MethodGet, "/services", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result []map[string]any
	decodeJSON(t, rec.Body.Bytes(), &result)
	for _, svc := range result {
		if title, _ := svc["title"].(string); title == "List /etc" {
			if accessible, _ := svc["accessible"].(bool); !accessible {
				t.Error("alice should have access to 'List /etc'")
			}
		}
	}
}

func TestGetMeWithoutSession(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	rec := doRequest(e, http.MethodGet, "/me", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "null" {
		t.Errorf("expected 'null', got '%s'", rec.Body.String())
	}
}

func TestGetMeAfterLogin(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	cookie := loginAs(t, e, "alice", "changeme1", false)
	rec := doRequest(e, http.MethodGet, "/me", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result map[string]any
	decodeJSON(t, rec.Body.Bytes(), &result)
	if result["username"] != "alice" {
		t.Errorf("expected username 'alice', got %v", result["username"])
	}
}

func TestLogin(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	tests := []struct {
		name     string
		body     string
		wantCode int
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{name: "valid credentials", body: `{"username":"alice","password":"changeme1"}`, wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var result map[string]any
				decodeJSON(t, rec.Body.Bytes(), &result)
				if result["username"] != "alice" {
					t.Errorf("expected username alice, got %v", result["username"])
				}
			},
		},
		{name: "invalid password", body: `{"username":"alice","password":"wrong"}`, wantCode: http.StatusUnauthorized},
		{name: "remember me sets persistent cookie", body: `{"username":"alice","password":"changeme1","rememberMe":true}`, wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				cookie := extractSessionCookie(rec)
				if cookie == nil {
					t.Fatal("expected session cookie")
				}
				if cookie.MaxAge <= 0 {
					t.Errorf("expected persistent cookie, got MaxAge=%d", cookie.MaxAge)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequestWithBody(e, http.MethodPost, "/login", tt.body, nil)
			if rec.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, rec.Code, rec.Body.String())
			}
			if tt.check != nil {
				tt.check(t, rec)
			}
		})
	}
}

func TestLogout(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	cookie := loginAs(t, e, "alice", "changeme1", false)
	rec := doRequest(e, http.MethodPost, "/logout", cookie)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestRunServiceReturnsJobId(t *testing.T) {
	e, _, store := setupTestEnv(t)
	cookie := loginAs(t, e, "alice", "changeme1", false)
	jobID := createJob(t, e, cookie)

	var j *job.Job
	var ok bool
	for i := 0; i < 20; i++ {
		j, ok = store.Get(jobID)
		if ok && (j.Status == job.StatusDone || j.Status == job.StatusRunning || j.Status == job.StatusError) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ok {
		t.Fatalf("job %s not found", jobID)
	}
}

func TestGetJobRunning(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	cookie := loginAs(t, e, "alice", "changeme1", false)
	jobID := createJob(t, e, cookie)

	rec := doRequest(e, http.MethodGet, "/jobs/"+jobID, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetJobCompleted(t *testing.T) {
	e, _, store := setupTestEnv(t)
	store.Set("test-job", &job.Job{Status: job.StatusDone, Title: "Test", Stdout: "hello"})

	rec := doRequest(e, http.MethodGet, "/jobs/test-job", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result map[string]any
	decodeJSON(t, rec.Body.Bytes(), &result)
	if result["status"] != job.StatusDone {
		t.Errorf("expected status 'done', got %v", result["status"])
	}
	if result["stdout"] != "hello" {
		t.Errorf("expected stdout 'hello', got %v", result["stdout"])
	}
}

func TestGetJobUnknown(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	rec := doRequest(e, http.MethodGet, "/jobs/unknown-id", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	var result map[string]string
	decodeJSON(t, rec.Body.Bytes(), &result)
	if result["error"] != "Unknown job" {
		t.Errorf("expected 'Unknown job', got '%s'", result["error"])
	}
}

func TestRunServiceNotFound(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	cookie := loginAs(t, e, "alice", "changeme1", false)
	rec := doRequest(e, http.MethodPost, "/run/999", cookie)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	var result map[string]string
	decodeJSON(t, rec.Body.Bytes(), &result)
	if result["error"] != "Unknown service" {
		t.Errorf("expected 'Unknown service', got '%s'", result["error"])
	}
}

func findServiceIndex(t *testing.T, cfg *config.Config, title string) int {
	t.Helper()
	for i, svc := range cfg.Flatten() {
		if svc.Title == title {
			return i
		}
	}
	t.Fatalf("service %q not found", title)
	return -1
}

func TestRunServiceForbidden(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	cookie := loginAs(t, e, "guest", "changeme3", false)

	cfg := buildTestConfig()
	restrictedIndex := findServiceIndex(t, cfg, "Admin only")

	rec := doRequest(e, http.MethodPost, "/run/"+strconv.Itoa(restrictedIndex), cookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginRateLimit(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"alice","password":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "10.99.99.1")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: expected 401, got %d", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"alice","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "10.99.99.1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after rate limit, got %d", rec.Code)
	}
}

func degradedEnv(t *testing.T) *echo.Echo {
	t.Helper()
	cfg := &config.Config{
		Global: &config.GlobalConfig{Port: ptrOf(99999)},
		Sections: []config.SectionConfig{{
			Title: "S", Services: []config.ServiceConfig{{Title: "T", Command: config.CommandValue{"echo"}}},
		}},
	}
	res := &config.LoadResult{Config: cfg, Issues: config.Validate(cfg)}
	store := job.NewStore()
	sec, err := auth.GenerateSessionSecret()
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(res, store, auth.NewSessionStore(sec), context.Background())
	e := echo.New()
	h.RegisterRoutes(e)
	return e
}

func TestDegradedBehaviors(t *testing.T) {
	e := degradedEnv(t)
	e.GET("/", func(c *echo.Context) error {
		return c.String(http.StatusOK, "<html>index</html>")
	})
	e.GET("/other", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/notfound", func(c *echo.Context) error {
		return c.String(http.StatusNotFound, "not found")
	})

	tests := []struct {
		name     string
		method   string
		path     string
		wantCode int
	}{
		{"degraded index returns 503", http.MethodGet, "/", http.StatusServiceUnavailable},
		{"other path not affected", http.MethodGet, "/other", http.StatusOK},
		{"non-200 not overridden", http.MethodGet, "/notfound", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Errorf("expected %d, got %d", tt.wantCode, rec.Code)
			}
		})
	}
}

func TestHealthyIndexReturns200(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	e.GET("/", func(c *echo.Context) error {
		return c.String(http.StatusOK, "<html>index</html>")
	})
	rec := doRequest(e, http.MethodGet, "/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestDegradedEndpoints(t *testing.T) {
	e := degradedEnv(t)
	tests := []struct {
		name     string
		method   string
		path     string
		wantCode int
		check    func(t *testing.T, body []byte)
	}{
		{name: "healthz returns 503", method: http.MethodGet, path: "/healthz", wantCode: http.StatusServiceUnavailable,
			check: func(t *testing.T, body []byte) {
				var result struct {
					Status string   `json:"status"`
					Issues []string `json:"issues"`
				}
				decodeJSON(t, body, &result)
				if result.Status != "error" || len(result.Issues) == 0 {
					t.Errorf("expected status=error with issues, got %+v", result)
				}
			},
		},
		{name: "services returns empty array", method: http.MethodGet, path: "/services", wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				if strings.TrimSpace(string(body)) != "[]" {
					t.Errorf("expected [], got %s", body)
				}
			},
		},
		{name: "config has degraded flag and issues", method: http.MethodGet, path: "/config", wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				var result struct {
					Title    string   `json:"title"`
					Degraded bool     `json:"degraded"`
					Issues   []string `json:"issues"`
				}
				decodeJSON(t, body, &result)
				if !result.Degraded {
					t.Error("expected degraded=true")
				}
				if result.Title != "Estro" {
					t.Errorf("expected title Estro, got %s", result.Title)
				}
				if len(result.Issues) == 0 {
					t.Error("expected non-empty issues")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(e, tt.method, tt.path, nil)
			if rec.Code != tt.wantCode {
				t.Errorf("expected %d, got %d", tt.wantCode, rec.Code)
			}
			if tt.check != nil {
				tt.check(t, rec.Body.Bytes())
			}
		})
	}
}

func TestListServicesRestrictedHiddenFromUnauthorized(t *testing.T) {
	e, _, _ := setupTestEnvWithConfig(t, buildRestrictedTestConfig())
	rec := doRequest(e, http.MethodGet, "/services", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result []map[string]any
	decodeJSON(t, rec.Body.Bytes(), &result)
	for _, svc := range result {
		if title, _ := svc["title"].(string); title == "Admin tool" {
			t.Error("restricted service 'Admin tool' should not appear for unauthenticated user")
		}
	}
}

func TestListServicesRestrictedVisibleForAllowedUser(t *testing.T) {
	e, _, _ := setupTestEnvWithConfig(t, buildRestrictedTestConfig())
	cookie := loginAs(t, e, "alice", "changeme1", false)
	rec := doRequest(e, http.MethodGet, "/services", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result []map[string]any
	decodeJSON(t, rec.Body.Bytes(), &result)
	found := false
	for _, svc := range result {
		if title, _ := svc["title"].(string); title == "Admin tool" {
			found = true
		}
	}
	if !found {
		t.Error("restricted service 'Admin tool' should appear for alice (admin)")
	}
}

func TestListServicesRestrictedFalseVisibleButNotAccessible(t *testing.T) {
	e, _, _ := setupTestEnvWithConfig(t, buildRestrictedTestConfig())
	cookie := loginAs(t, e, "guest", "changeme3", false)
	rec := doRequest(e, http.MethodGet, "/services", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result []map[string]any
	decodeJSON(t, rec.Body.Bytes(), &result)
	found := false
	for _, svc := range result {
		if title, _ := svc["title"].(string); title == "Visible admin tool" {
			found = true
			if accessible, _ := svc["accessible"].(bool); accessible {
				t.Error("restricted=false service should be visible but not accessible to guest")
			}
		}
	}
	if !found {
		t.Error("restricted=false service should be visible to guest even if not accessible")
	}
}

func TestRunServiceRestrictedHiddenReturns404(t *testing.T) {
	e, _, _ := setupTestEnvWithConfig(t, buildRestrictedTestConfig())
	cookie := loginAs(t, e, "guest", "changeme3", false)
	rec := doRequest(e, http.MethodPost, "/run/1", cookie)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	var result map[string]string
	decodeJSON(t, rec.Body.Bytes(), &result)
	if result["error"] != "Unknown service" {
		t.Errorf("expected 'Unknown service', got '%s'", result["error"])
	}
}

func TestExecuteAsyncStderr(t *testing.T) {
	tests := []struct {
		name       string
		cmd        string
		wantStatus string
		wantStderr string
	}{
		{"failure with stderr keeps the real stderr", "echo boom >&2; exit 1", job.StatusError, "boom"},
		{"failure without stderr falls back to the error", "exit 3", job.StatusError, "exit status 3"},
		{"success has empty stderr", "echo ok", job.StatusDone, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{jobs: job.NewStore(), cmdCtx: context.Background()}
			svc := config.FlatService{Title: "svc", Timeout: 5}
			h.executeAsync("jid", svc, tt.cmd)

			j, ok := h.jobs.Get("jid")
			if !ok {
				t.Fatal("job not stored")
			}
			if j.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", j.Status, tt.wantStatus)
			}
			if j.Stderr != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", j.Stderr, tt.wantStderr)
			}
		})
	}
}

func TestSessionSliding(t *testing.T) {
	tests := []struct {
		name       string
		sessionTTL int // 0 = unlimited
	}{
		{"unlimited TTL slides cookie", 0},
		{"explicit TTL slides cookie", 720},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := buildTestConfig()
			cfg.Global.SessionTTL = ptrOf(tt.sessionTTL)
			e, _, _ := setupTestEnvWithConfig(t, cfg)

			cookie := loginAs(t, e, "alice", "changeme1", true)

			rec := doRequest(e, http.MethodGet, "/me", cookie)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			if !hasSessionCookie(rec) {
				t.Fatal("expected refreshed session cookie from sliding middleware")
			}

			freshCookie := extractSessionCookie(rec)
			rec2 := doRequest(e, http.MethodGet, "/me", freshCookie)
			var result map[string]any
			decodeJSON(t, rec2.Body.Bytes(), &result)
			if result["username"] != "alice" {
				t.Errorf("expected alice after sliding refresh, got %v", result["username"])
			}
		})
	}
}

func TestSessionSlidingNoOpForSessionOnly(t *testing.T) {
	cfg := buildTestConfig()
	e, _, _ := setupTestEnvWithConfig(t, cfg)

	// Login without rememberMe → session-only cookie
	cookie := loginAs(t, e, "alice", "changeme1", false)

	rec := doRequest(e, http.MethodGet, "/me", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if hasSessionCookie(rec) {
		t.Error("session-only cookie should not be refreshed to persistent by sliding middleware")
	}
}

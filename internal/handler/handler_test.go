package handler

import (
	"context"
	"encoding/json"
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
	cookie := findSessionCookie(rec)
	if cookie == nil {
		t.Fatal("expected session cookie after login")
	}
	return cookie
}

func findSessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "connect.sid" {
			return c
		}
	}
	return nil
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

func TestGetConfigDefaultTitle(t *testing.T) {
	cfg := &config.Config{
		Global: &config.GlobalConfig{Title: ptrOf("Estro"), Hostname: ptrOf("0.0.0.0"), Port: ptrOf(3000)},
		Users:  map[string]*config.UserConfig{"testuser": {Password: "$2y$10$hash"}},
		Sections: []config.SectionConfig{{
			Title: "Test", Services: []config.ServiceConfig{{Title: "Svc", Command: config.CommandValue{"echo hi"}}},
		}},
	}
	res := loadConfig(t, cfg)
	resp := res.Config.GetConfigResponse()
	if resp.Title != "Estro" {
		t.Errorf("expected default title 'Estro', got %s", resp.Title)
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

func TestLoginValidCredentials(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	rec := doRequestWithBody(e, http.MethodPost, "/login", `{"username":"alice","password":"changeme1"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	decodeJSON(t, rec.Body.Bytes(), &result)
	if result["username"] != "alice" {
		t.Errorf("expected username 'alice', got %v", result["username"])
	}
}

func TestLoginInvalidPassword(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	rec := doRequestWithBody(e, http.MethodPost, "/login", `{"username":"alice","password":"wrong"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLoginRememberMe(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	rec := doRequestWithBody(e, http.MethodPost, "/login", `{"username":"alice","password":"changeme1","rememberMe":true}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	cookie := findSessionCookie(rec)
	if cookie == nil {
		t.Fatal("expected session cookie to be set")
	}
	if cookie.MaxAge <= 0 {
		t.Errorf("expected persistent cookie with MaxAge > 0, got MaxAge=%d", cookie.MaxAge)
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
	rec := doRequest(e, http.MethodPost, "/run/0", cookie)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	decodeJSON(t, rec.Body.Bytes(), &result)
	jobID, _ := result["jobId"].(string)
	if jobID == "" {
		t.Fatal("expected jobId in response")
	}

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
	rec := doRequest(e, http.MethodPost, "/run/0", cookie)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	var runResult map[string]any
	decodeJSON(t, rec.Body.Bytes(), &runResult)
	jobID, _ := runResult["jobId"].(string)

	rec2 := doRequest(e, http.MethodGet, "/jobs/"+jobID, cookie)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
}

func TestGetJobCompleted(t *testing.T) {
	store := job.NewStore()
	store.Set("test-job", &job.Job{Status: job.StatusDone, Title: "Test", Stdout: "hello"})

	res := loadConfig(t, buildTestConfig())
	sessionSecret, _ := auth.GenerateSessionSecret()
	sessionStore := auth.NewSessionStore(sessionSecret)
	h := NewHandler(res, store, sessionStore, context.Background())
	e := echo.New()
	e.GET("/jobs/:id", h.getJob)

	req := httptest.NewRequest(http.MethodGet, "/jobs/test-job", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
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

func TestRunServiceForbidden(t *testing.T) {
	e, _, _ := setupTestEnv(t)
	cookie := loginAs(t, e, "guest", "changeme3", false)

	cfg := buildTestConfig()
	services := cfg.Flatten()
	restrictedIndex := -1
	for i, svc := range services {
		if !svc.IsAccessible("guest") {
			restrictedIndex = i
			break
		}
	}
	if restrictedIndex == -1 {
		t.Skip("no restricted services found to test")
	}

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

func TestHealthzDegraded(t *testing.T) {
	e := degradedEnv(t)
	rec := doRequest(e, http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	var body struct {
		Status string   `json:"status"`
		Issues []string `json:"issues"`
	}
	decodeJSON(t, rec.Body.Bytes(), &body)
	if body.Status != "error" || len(body.Issues) == 0 {
		t.Fatalf("expected status=error with issues, got %+v", body)
	}
}

func TestConfigDegradedIssues(t *testing.T) {
	e := degradedEnv(t)
	rec := doRequest(e, http.MethodGet, "/config", nil)
	var body struct {
		Degraded bool     `json:"degraded"`
		Issues   []string `json:"issues"`
	}
	decodeJSON(t, rec.Body.Bytes(), &body)
	if !body.Degraded || len(body.Issues) == 0 {
		t.Fatalf("expected degraded with issues, got %+v", body)
	}
}

func TestConfigDegradedKeepsDefaultTitle(t *testing.T) {
	e := degradedEnv(t)
	rec := doRequest(e, http.MethodGet, "/config", nil)
	var body struct {
		Title    string `json:"title"`
		Degraded bool   `json:"degraded"`
	}
	decodeJSON(t, rec.Body.Bytes(), &body)
	if !body.Degraded || body.Title != "Estro" {
		t.Fatalf("expected degraded with default title 'Estro', got %+v", body)
	}
}

func TestServicesDegradedEmpty(t *testing.T) {
	e := degradedEnv(t)
	rec := doRequest(e, http.MethodGet, "/services", nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("expected 200 []; got %d %q", rec.Code, rec.Body.String())
	}
}

func TestListServices_RestrictedHiddenFromUnauthorized(t *testing.T) {
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

func TestListServices_RestrictedVisibleForAllowedUser(t *testing.T) {
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

func TestListServices_RestrictedFalse_VisibleButNotAccessible(t *testing.T) {
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

func TestRunService_RestrictedHidden_Returns404(t *testing.T) {
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

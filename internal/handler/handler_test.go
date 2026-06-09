package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo/v5"
	echoMiddleware "github.com/labstack/echo/v5/middleware"
	"github.com/spaghetti-coder/estro/internal/auth"
	"github.com/spaghetti-coder/estro/internal/config"
	"github.com/spaghetti-coder/estro/internal/job"
	appMiddleware "github.com/spaghetti-coder/estro/internal/middleware"
)

const testConfigYAML = `---
global:
  title: Estro
  subtitle: Test suite
  hostname: 127.0.0.1
  port: 3000
  timeout: 30
  confirm: true
  restricted: false
users:
  alice:
    password: '$2a$04$RaT2a8ho6k.L3WboREnlDOzOFvVRT4BZ27qHvoYjWIy.f86sEzRCi'
    groups: [admins]
  bob:
    password: '$2a$04$eFgD5RBrhd9knn7y65gOo.puc0zS7aU/5vE9rtR2Zoh4W.PQN2Qti'
    groups: [admins, family]
  guest:
    password: '$2a$04$BHALOlcRQPMb435x.vD/0OTDO0fqn8e6iVb4BkIgVKuirpah7u0tW'
sections:
  - title: Public Info
    collapsable: false
    columns: 1
    services:
      - title: Uptime
        command: uptime
      - title: Date
        command: date
        confirm: false
  - title: Admin
    allowed: [admins]
    columns: 4
    services:
      - title: List /etc
        command: ls -lah /etc
      - title: Who
        command: who
        confirm: false
  - title: Mixed Access
    allowed: [admins]
    services:
      - title: Public status
        command: uptime
        allowed: []
        confirm: false
      - title: Admin only
        command: id
      - title: Guest allowed
        command: date
        allowed: [guest]
`

func loadTestConfig(t *testing.T) *config.Config {
	t.Helper()
	tmp, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString(testConfigYAML); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		t.Fatal(err)
	}
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmp.Name()) }()

	res := config.Load(tmp.Name())
	if !res.Healthy() {
		t.Fatalf("failed to load test config: %v", res.IssueStrings())
	}
	return res.Config
}

func setupTestEnvWithConfig(t *testing.T, yamlContent string) (*echo.Echo, *Handler, *job.Store, sessions.Store) {
	t.Helper()
	tmp, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString(yamlContent); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		t.Fatal(err)
	}
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmp.Name()) }()

	res := config.Load(tmp.Name())
	if !res.Healthy() {
		t.Fatalf("failed to load test config: %v", res.IssueStrings())
	}

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

	return e, h, store, sessionStore
}

func setupTestEnv(t *testing.T) (*echo.Echo, *Handler, *job.Store, sessions.Store) {
	t.Helper()
	return setupTestEnvWithConfig(t, testConfigYAML)
}

func TestGetConfig(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["title"] != "Estro" {
		t.Errorf("expected title 'Estro', got %v", result["title"])
	}
	if result["subtitle"] != "Test suite" {
		t.Errorf("expected subtitle 'Test suite', got %v", result["subtitle"])
	}
	users, ok := result["users"].([]interface{})
	if !ok {
		t.Fatalf("expected users array, got %v", result["users"])
	}
	if len(users) != 3 {
		t.Errorf("expected 3 users, got %d", len(users))
	}
	if result["degraded"] != false {
		t.Errorf("expected degraded=false on healthy config, got %v", result["degraded"])
	}
}

func TestGetConfigDefaultTitle(t *testing.T) {
	noTitleConfig := `---
users:
  testuser:
    password: '$2y$10$hash'
sections:
  - title: Test
    services:
      - title: Svc
        command: echo hi
`
	tmp, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString(noTitleConfig); err != nil {
		t.Fatal(err)
	}
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmp.Name()) }()

	res := config.Load(tmp.Name())
	if !res.Healthy() {
		t.Fatalf("failed to load config: %v", res.IssueStrings())
	}
	resp := res.Config.GetConfigResponse()
	if resp.Title != "Estro" {
		t.Errorf("expected default title 'Estro', got %s", resp.Title)
	}
}

func TestListServicesUnauthenticated(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result []interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if len(result) == 0 {
		t.Error("expected services, got empty array")
	}
}

func TestListServicesAuthenticatedAlice(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	cookie := loginAs(t, e, "alice", "changeme1", false)

	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	for _, svc := range result {
		title, _ := svc["title"].(string)
		accessible, _ := svc["accessible"].(bool)
		if title == "List /etc" && !accessible {
			t.Errorf("alice should have access to 'List /etc', but accessible=false")
		}
	}
}

func TestGetMeWithoutSession(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := strings.TrimSpace(rec.Body.String())
	if body != "null" {
		t.Errorf("expected 'null', got '%s'", body)
	}
}

func TestGetMeAfterLogin(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	cookie := loginAs(t, e, "alice", "changeme1", false)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["username"] != "alice" {
		t.Errorf("expected username 'alice', got %v", result["username"])
	}
}

func TestLoginValidCredentials(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	body := `{"username":"alice","password":"changeme1"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["username"] != "alice" {
		t.Errorf("expected username 'alice', got %v", result["username"])
	}
}

func TestLoginInvalidPassword(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	body := `{"username":"alice","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLoginRememberMe(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	body := `{"username":"alice","password":"changeme1","rememberMe":true}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	cookie := findSessionCookie(rec)
	if cookie == nil {
		t.Fatal("expected session cookie to be set")
	}
	if cookie.MaxAge <= 0 {
		t.Errorf("expected persistent cookie with MaxAge > 0 for rememberMe, got MaxAge=%d", cookie.MaxAge)
	}
}

func TestLogout(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	cookie := loginAs(t, e, "alice", "changeme1", false)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestRunServiceReturnsJobId(t *testing.T) {
	e, _, store, _ := setupTestEnv(t)

	cookie := loginAs(t, e, "alice", "changeme1", false)

	req := httptest.NewRequest(http.MethodPost, "/run/0", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["jobId"] == nil || result["jobId"] == "" {
		t.Error("expected jobId in response")
	}

	jobID, _ := result["jobId"].(string)

	var j *job.Job
	var ok bool
	for i := 0; i < 20; i++ {
		j, ok = store.Get(jobID)
		if ok {
			if j.Status == job.StatusDone || j.Status == job.StatusError {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !ok {
		t.Fatalf("expected job %s to exist", jobID)
	}
	if j.Status != job.StatusDone && j.Status != job.StatusRunning && j.Status != job.StatusError {
		t.Errorf("expected job status done/running/error, got %s", j.Status)
	}
}

func TestGetJobRunning(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	cookie := loginAs(t, e, "alice", "changeme1", false)

	req := httptest.NewRequest(http.MethodPost, "/run/0", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	var runResult map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &runResult); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	jobID, _ := runResult["jobId"].(string)

	req2 := httptest.NewRequest(http.MethodGet, "/jobs/"+jobID, nil)
	req2.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
}

func TestGetJobCompleted(t *testing.T) {
	store := job.NewStore()
	store.Set("test-job", &job.Job{Status: job.StatusDone, Title: "Test", Stdout: "hello"})

	cfg := loadTestConfig(t)
	sessionSecret, err := auth.GenerateSessionSecret()
	if err != nil {
		t.Fatal(err)
	}
	sessionStore := auth.NewSessionStore(sessionSecret)
	h := NewHandler(&config.LoadResult{Config: cfg}, store, sessionStore, context.Background())

	e := echo.New()
	e.GET("/jobs/:id", h.getJob)

	req := httptest.NewRequest(http.MethodGet, "/jobs/test-job", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["status"] != job.StatusDone {
		t.Errorf("expected status 'done', got %v", result["status"])
	}
	if result["stdout"] != "hello" {
		t.Errorf("expected stdout 'hello', got %v", result["stdout"])
	}
}

func TestGetJobUnknown(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/jobs/unknown-id", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	var result map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["error"] != "Unknown job" {
		t.Errorf("expected 'Unknown job', got '%s'", result["error"])
	}
}

func TestRunServiceNotFound(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	cookie := loginAs(t, e, "alice", "changeme1", false)

	req := httptest.NewRequest(http.MethodPost, "/run/999", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	var result map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["error"] != "Unknown service" {
		t.Errorf("expected 'Unknown service', got '%s'", result["error"])
	}
}

func TestRunServiceForbidden(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	cookie := loginAs(t, e, "guest", "changeme3", false)

	restrictedIndex := -1
	cfg := loadTestConfig(t)
	services := cfg.Flatten()
	for i, svc := range services {
		if !svc.IsAccessible("guest") {
			restrictedIndex = i
			break
		}
	}
	if restrictedIndex == -1 {
		t.Skip("no restricted services found to test")
	}

	req := httptest.NewRequest(http.MethodPost, "/run/"+strconv.Itoa(restrictedIndex), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for restricted service, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginRateLimit(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	for i := 0; i < 10; i++ {
		body := `{"username":"alice","password":"wrong"}`
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "10.0.0.1")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if i < 10 && rec.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: expected 401, got %d", i+1, rec.Code)
		}
	}

	body := `{"username":"alice","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after rate limit, got %d", rec.Code)
	}
}

func loginAs(t *testing.T, e *echo.Echo, username, password string, rememberMe bool) *http.Cookie {
	t.Helper()
	body := `{"username":"` + username + `","password":"` + password + `"}`
	if rememberMe {
		body = `{"username":"` + username + `","password":"` + password + `","rememberMe":true}`
	}
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

const restrictedTestConfigYAML = `---
global:
  title: Estro
  subtitle: Restricted test
  hostname: 127.0.0.1
  port: 3000
  timeout: 30
  confirm: true
  restricted: false
users:
  alice:
    password: '$2a$04$RaT2a8ho6k.L3WboREnlDOzOFvVRT4BZ27qHvoYjWIy.f86sEzRCi'
    groups: [admins]
  guest:
    password: '$2a$04$BHALOlcRQPMb435x.vD/0OTDO0fqn8e6iVb4BkIgVKuirpah7u0tW'
sections:
  - title: Public section
    services:
      - title: Uptime
        command: uptime
  - title: Admin section
    allowed: [admins]
    restricted: true
    services:
      - title: Admin tool
        command: id
  - title: Visible restricted section
    allowed: [admins]
    restricted: false
    services:
      - title: Visible admin tool
        command: whoami
`

func TestListServices_RestrictedHiddenFromUnauthorized(t *testing.T) {
	e, _, _, _ := setupTestEnvWithConfig(t, restrictedTestConfigYAML)

	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	for _, svc := range result {
		title, _ := svc["title"].(string)
		if title == "Admin tool" {
			t.Error("restricted service 'Admin tool' should not appear for unauthenticated user")
		}
	}
}

func TestListServices_RestrictedVisibleForAllowedUser(t *testing.T) {
	e, _, _, _ := setupTestEnvWithConfig(t, restrictedTestConfigYAML)

	cookie := loginAs(t, e, "alice", "changeme1", false)

	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	found := false
	for _, svc := range result {
		title, _ := svc["title"].(string)
		if title == "Admin tool" {
			found = true
		}
	}
	if !found {
		t.Error("restricted service 'Admin tool' should appear for alice (admin)")
	}
}

func TestListServices_RestrictedFalse_VisibleButNotAccessible(t *testing.T) {
	e, _, _, _ := setupTestEnvWithConfig(t, restrictedTestConfigYAML)

	cookie := loginAs(t, e, "guest", "changeme3", false)

	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	found := false
	for _, svc := range result {
		title, _ := svc["title"].(string)
		if title == "Visible admin tool" {
			found = true
			accessible, _ := svc["accessible"].(bool)
			if accessible {
				t.Error("restricted=false service should be visible but not accessible to guest")
			}
		}
	}
	if !found {
		t.Error("restricted=false service should be visible to guest even if not accessible")
	}
}

func TestRunService_RestrictedHidden_Returns404(t *testing.T) {
	e, _, _, _ := setupTestEnvWithConfig(t, restrictedTestConfigYAML)

	cookie := loginAs(t, e, "guest", "changeme3", false)

	req := httptest.NewRequest(http.MethodPost, "/run/1", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for restricted-hidden service, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["error"] != "Unknown service" {
		t.Errorf("expected 'Unknown service', got '%s'", result["error"])
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatal(err)
	}
	_ = tmp.Close()
	t.Cleanup(func() { _ = os.Remove(tmp.Name()) })
	return tmp.Name()
}

func degradedEnv(t *testing.T) *echo.Echo {
	t.Helper()
	res := config.Load(writeTempConfig(t, "global:\n  port: 99999\n")) // invalid port, no sections => degraded
	e := echo.New()
	store := job.NewStore()
	sec, err := auth.GenerateSessionSecret()
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(res, store, auth.NewSessionStore(sec), context.Background())
	h.RegisterRoutes(e)
	return e
}

func TestDegradedIndexReturns503(t *testing.T) {
	e := degradedEnv(t)
	// Simulate the static file handler serving index.html with status 200.
	e.GET("/", func(c *echo.Context) error {
		return c.String(http.StatusOK, "<html>index</html>")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for GET / in degraded mode, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "<html>index</html>") {
		t.Errorf("expected page content to be served, got %q", body)
	}
}

func TestDegradedIndexOtherPathsNotAffected(t *testing.T) {
	e := degradedEnv(t)
	e.GET("/other", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/other", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for non-index path in degraded mode, got %d", rec.Code)
	}
}

func TestDegradedIndexNon200NotOverridden(t *testing.T) {
	e := degradedEnv(t)
	e.GET("/", func(c *echo.Context) error {
		return c.String(http.StatusNotFound, "not found")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 to remain unchanged, got %d", rec.Code)
	}
}

func TestHealthyIndexReturns200(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)
	e.GET("/", func(c *echo.Context) error {
		return c.String(http.StatusOK, "<html>index</html>")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET / in healthy mode, got %d", rec.Code)
	}
}

func TestHealthzDegraded(t *testing.T) {
	e := degradedEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	var body struct {
		Status string   `json:"status"`
		Issues []string `json:"issues"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "error" || len(body.Issues) == 0 {
		t.Fatalf("expected status=error with issues, got %+v", body)
	}
}

func TestConfigDegradedIssues(t *testing.T) {
	e := degradedEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var body struct {
		Degraded bool     `json:"degraded"`
		Issues   []string `json:"issues"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Degraded || len(body.Issues) == 0 {
		t.Fatalf("expected degraded with issues, got %+v", body)
	}
}

func TestConfigDegradedKeepsDefaultTitle(t *testing.T) {
	e := degradedEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var body struct {
		Title    string `json:"title"`
		Degraded bool   `json:"degraded"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Degraded || body.Title != "Estro" {
		t.Fatalf("expected degraded with default title 'Estro', got %+v", body)
	}
}

func TestServicesDegradedEmpty(t *testing.T) {
	e := degradedEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("expected 200 []; got %d %q", rec.Code, rec.Body.String())
	}
}

func findSessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "connect.sid" {
			return c
		}
	}
	return nil
}

func TestExecuteAsyncStderr(t *testing.T) {
	tests := []struct {
		name       string
		cmd        string
		wantStatus string
		wantStderr string
	}{
		{
			name:       "failure with stderr keeps the real stderr",
			cmd:        "echo boom >&2; exit 1",
			wantStatus: job.StatusError,
			wantStderr: "boom",
		},
		{
			name:       "failure without stderr falls back to the error",
			cmd:        "exit 3",
			wantStatus: job.StatusError,
			wantStderr: "exit status 3",
		},
		{
			name:       "success has empty stderr",
			cmd:        "echo ok",
			wantStatus: job.StatusDone,
			wantStderr: "",
		},
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

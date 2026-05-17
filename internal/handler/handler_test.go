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
    password: '$2y$10$6c9tQEpF4w2Ev9XCLH2pauAawy2874wwgN5jCTyrYMYclVlTTNIs2'
    groups: [admins]
  bob:
    password: '$2y$10$IMxjm44TuD0J0JAHeqzcdOufsumn.G2Y.ZS15EOQq9vbmCg48Zvnm'
    groups: [admins, family]
  guest:
    password: '$2y$10$VIRU2eYVGPRE1qf5PqDCQuSt9RPLd/0E2HxjmZeU6ELIgsFFmQn/C'
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

	cfg, err := config.Load(tmp.Name())
	if err != nil {
		t.Fatalf("failed to load test config: %v", err)
	}
	return cfg
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

	cfg, err := config.Load(tmp.Name())
	if err != nil {
		t.Fatalf("failed to load test config: %v", err)
	}

	store := job.NewStore()
	sessionSecret := auth.GenerateSessionSecret()
	sessionStore := auth.NewSessionStore(sessionSecret)
	h := NewHandler(cfg, store, sessionStore, sessionSecret, context.Background())

	e := echo.New()
	e.Use(appMiddleware.SecurityMiddleware("default-src 'self'"))
	e.Use(appMiddleware.FaviconCORS())
	e.Use(auth.SessionMiddleware(sessionStore))
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

	cfg, err := config.Load(tmp.Name())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	resp := cfg.GetConfigResponse()
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
	groups, ok := result["groups"].([]interface{})
	if !ok {
		t.Errorf("expected groups array, got %v", result["groups"])
	} else if len(groups) != 1 || groups[0] != "admins" {
		t.Errorf("expected groups [admins], got %v", groups)
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

func TestLoginEmptyUsername(t *testing.T) {
	e, _, _, _ := setupTestEnv(t)

	body := `{"username":"","password":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
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

	time.Sleep(2 * time.Second)

	jobID, _ := result["jobId"].(string)
	j, ok := store.Get(jobID)
	if !ok {
		t.Fatalf("expected job %s to exist", jobID)
	}
	if j.Status != "done" && j.Status != "running" && j.Status != "error" {
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
	store.Set("test-job", &job.Job{Status: "done", Title: "Test", Stdout: "hello"})

	cfg := loadTestConfig(t)
	sessionSecret := auth.GenerateSessionSecret()
	sessionStore := auth.NewSessionStore(sessionSecret)
	h := NewHandler(cfg, store, sessionStore, sessionSecret, context.Background())

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
	if result["status"] != "done" {
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
		if !svc.IsAccessible("guest", cfg.Users) {
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
    password: '$2y$10$6c9tQEpF4w2Ev9XCLH2pauAawy2874wwgN5jCTyrYMYclVlTTNIs2'
    groups: [admins]
  guest:
    password: '$2y$10$VIRU2eYVGPRE1qf5PqDCQuSt9RPLd/0E2HxjmZeU6ELIgsFFmQn/C'
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

func findSessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "connect.sid" {
			return c
		}
	}
	return nil
}

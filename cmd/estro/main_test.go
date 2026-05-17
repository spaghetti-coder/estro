package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/spaghetti-coder/estro"
	appMiddleware "github.com/spaghetti-coder/estro/internal/middleware"
)

func setupStaticTestServer(t *testing.T) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.Use(appMiddleware.SecurityMiddleware("default-src 'self'"))
	e.Use(appMiddleware.FaviconCORS())

	subFS, err := fs.Sub(estro.StaticFS, "public")
	if err != nil {
		t.Fatalf("failed to create sub filesystem: %v", err)
	}
	e.StaticFS("/", subFS)

	return e
}

func TestStaticRootReturnsHTML(t *testing.T) {
	e := setupStaticTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html content type, got %q", ct)
	}
}

func TestStaticUIJSReturnsJS(t *testing.T) {
	e := setupStaticTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/ui.js", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/javascript") && !strings.Contains(ct, "text/javascript") {
		t.Errorf("expected javascript content type, got %q", ct)
	}
}

func TestStaticStylesCSSReturnsCSS(t *testing.T) {
	e := setupStaticTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/styles.css", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/css") {
		t.Errorf("expected text/css content type, got %q", ct)
	}
}

func TestStaticFaviconSVGWithCORS(t *testing.T) {
	e := setupStaticTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	acao := rec.Header().Get("Access-Control-Allow-Origin")
	if acao != "*" {
		t.Errorf("expected Access-Control-Allow-Origin '*', got %q", acao)
	}
	corp := rec.Header().Get("Cross-Origin-Resource-Policy")
	if corp != "cross-origin" {
		t.Errorf("expected Cross-Origin-Resource-Policy 'cross-origin', got %q", corp)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "svg") && !strings.Contains(ct, "xml") && !strings.Contains(ct, "image/") {
		t.Errorf("expected SVG content type, got %q", ct)
	}
}

func TestStaticFaviconCORSNotOnOtherRoutes(t *testing.T) {
	e := setupStaticTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/ui.js", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	acao := rec.Header().Get("Access-Control-Allow-Origin")
	if acao != "" {
		t.Errorf("expected no Access-Control-Allow-Origin on /ui.js, got %q", acao)
	}
}

func TestEmbeddedFilesExist(t *testing.T) {
	files := []string{"index.html", "ui.js", "styles.css", "favicon.svg"}
	subFS, err := fs.Sub(estro.StaticFS, "public")
	if err != nil {
		t.Fatalf("failed to create sub filesystem: %v", err)
	}
	for _, f := range files {
		if _, err := fs.Stat(subFS, f); err != nil {
			t.Errorf("expected file %q in embedded FS, got error: %v", f, err)
		}
	}
}

func TestRunHashWithPassword(t *testing.T) {
	exitCode := runHash([]string{"testpass"})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
}

func TestRunHashTooManyArgs(t *testing.T) {
	exitCode := runHash([]string{"a", "b"})
	if exitCode != 1 {
		t.Fatalf("expected exit code 1 for too many args, got %d", exitCode)
	}
}

func TestRootReturnsIndexHTMLBody(t *testing.T) {
	e := setupStaticTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<!DOCTYPE") && !strings.Contains(strings.ToLower(body), "html") {
		t.Errorf("expected HTML body, got: %s", body[:min(len(body), 200)])
	}
}

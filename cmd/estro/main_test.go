package main

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/spaghetti-coder/estro"
	"github.com/spaghetti-coder/estro/internal/auth"
	appMiddleware "github.com/spaghetti-coder/estro/internal/middleware"
	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	auth.SetBcryptCost(bcrypt.MinCost)
	os.Exit(m.Run())
}

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

func TestStaticFileServing(t *testing.T) {
	e := setupStaticTestServer(t)

	tests := []struct {
		name       string
		route      string
		wantCT     string
		wantNoCORS bool
		wantHTML   bool
	}{
		{name: "ui.js returns javascript", route: "/ui.js", wantCT: "javascript", wantNoCORS: true},
		{name: "styles.css returns css", route: "/styles.css", wantCT: "text/css"},
		{name: "root returns html", route: "/", wantCT: "text/html", wantHTML: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.route, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			ct := rec.Header().Get("Content-Type")
			if !strings.Contains(ct, tt.wantCT) {
				t.Errorf("expected content type containing %q, got %q", tt.wantCT, ct)
			}
			if tt.wantNoCORS {
				if acao := rec.Header().Get("Access-Control-Allow-Origin"); acao != "" {
					t.Errorf("expected no Access-Control-Allow-Origin on %s, got %q", tt.route, acao)
				}
			}
			if tt.wantHTML {
				body := rec.Body.String()
				if !strings.Contains(body, "<!DOCTYPE") && !strings.Contains(strings.ToLower(body), "html") {
					t.Errorf("expected HTML body, got: %s", body[:min(len(body), 200)])
				}
			}
		})
	}
}

func TestStaticFaviconSVGReachable(t *testing.T) {
	e := setupStaticTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "svg") && !strings.Contains(ct, "xml") && !strings.Contains(ct, "image/") {
		t.Errorf("expected SVG content type, got %q", ct)
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

func TestRunHash(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		verifyPass string // non-empty: capture stdout and validate it as a bcrypt hash
	}{
		{name: "single password", args: []string{"testpass"}, wantExit: 0, verifyPass: "testpass"},
		{name: "too many args", args: []string{"a", "b"}, wantExit: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.verifyPass != "" {
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatalf("os.Pipe: %v", err)
				}
				old := os.Stdout
				os.Stdout = w

				exitCode := runHash(tt.args)

				os.Stdout = old
				if err := w.Close(); err != nil {
					t.Fatalf("close pipe writer: %v", err)
				}
				b, _ := io.ReadAll(r)
				out := strings.TrimSpace(string(b))

				if exitCode != tt.wantExit {
					t.Fatalf("expected exit code %d, got %d", tt.wantExit, exitCode)
				}
				if err := bcrypt.CompareHashAndPassword([]byte(out), []byte(tt.verifyPass)); err != nil {
					t.Fatalf("stdout %q is not a valid bcrypt hash for %q: %v", out, tt.verifyPass, err)
				}
				return
			}

			exitCode := runHash(tt.args)
			if exitCode != tt.wantExit {
				t.Fatalf("expected exit code %d, got %d", tt.wantExit, exitCode)
			}
		})
	}
}

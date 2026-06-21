package main

import (
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/spaghetti-coder/estro/internal/app"
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

	if err := app.MountStatic(e, ""); err != nil {
		t.Fatalf("failed to mount static files: %v", err)
	}

	return e
}

func TestParseServerFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		envConfig  string
		wantConfig string
		wantStatic string
	}{
		{
			name:       "env default",
			args:       []string{"estro"},
			envConfig:  "/env.yaml",
			wantConfig: "/env.yaml",
		},
		{
			name:       "flag overrides env",
			args:       []string{"estro", "-config", "/flag.yaml"},
			envConfig:  "/env.yaml",
			wantConfig: "/flag.yaml",
		},
		{
			name:       "flags set both",
			args:       []string{"estro", "-config", "/c.yaml", "-static-dir", "/static"},
			wantConfig: "/c.yaml",
			wantStatic: "/static",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldCL := flag.CommandLine
			t.Cleanup(func() { flag.CommandLine = oldCL })
			flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)

			t.Setenv("ESTRO_CONFIG", tt.envConfig)

			oldArgs := os.Args
			os.Args = tt.args
			t.Cleanup(func() { os.Args = oldArgs })

			f := parseServerFlags()

			if f.configPath != tt.wantConfig {
				t.Fatalf("configPath: want %q, got %q", tt.wantConfig, f.configPath)
			}
			if f.staticDir != tt.wantStatic {
				t.Fatalf("staticDir: want %q, got %q", tt.wantStatic, f.staticDir)
			}
		})
	}
}

func TestStaticFiles(t *testing.T) {
	e := setupStaticTestServer(t)

	tests := []struct {
		name     string
		route    string
		wantCT   string // substring expected in Content-Type; empty skips the check
		wantCORS bool   // expect Access-Control-Allow-Origin to be set
		wantHTML bool   // expect an HTML body
	}{
		{name: "ui.js returns javascript", route: "/ui.js", wantCT: "javascript"},
		{name: "styles.css returns css", route: "/styles.css", wantCT: "text/css"},
		{name: "root returns html", route: "/", wantCT: "text/html", wantHTML: true},
		{name: "index.html reachable", route: "/index.html", wantCT: "text/html", wantHTML: true},
		{name: "favicon.svg reachable with CORS", route: "/favicon.svg", wantCT: "svg", wantCORS: true},
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
			if tt.wantCT != "" && !strings.Contains(ct, tt.wantCT) {
				t.Errorf("expected content type containing %q, got %q", tt.wantCT, ct)
			}
			acao := rec.Header().Get("Access-Control-Allow-Origin")
			if tt.wantCORS && acao != "*" {
				t.Errorf("expected Access-Control-Allow-Origin=* on %s, got %q", tt.route, acao)
			}
			if !tt.wantCORS && acao != "" {
				t.Errorf("expected no Access-Control-Allow-Origin on %s, got %q", tt.route, acao)
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

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExit int
	}{
		{name: "server no config", args: []string{"estro"}, wantExit: 1},
		{name: "hash single arg", args: []string{"estro", "hash", "pw"}, wantExit: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldArgs := os.Args
			os.Args = tt.args
			t.Cleanup(func() { os.Args = oldArgs })

			oldCL := flag.CommandLine
			flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
			t.Cleanup(func() { flag.CommandLine = oldCL })

			t.Setenv("ESTRO_CONFIG", "")

			if got := run(tt.args); got != tt.wantExit {
				t.Fatalf("run() = %d, want %d", got, tt.wantExit)
			}
		})
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
				t.Cleanup(func() { os.Stdout = old })

				exitCode := runHash(tt.args)

				if err := w.Close(); err != nil {
					t.Fatalf("close pipe writer: %v", err)
				}
				b, err := io.ReadAll(r)
				if err != nil {
					t.Fatalf("read stdout: %v", err)
				}
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

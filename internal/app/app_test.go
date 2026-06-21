package app_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/spaghetti-coder/estro/internal/app"
)

func testdataPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", name)
}

func TestRun_StaticDirErrors(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	notDir := filepath.Join(tmp, "file")
	if err := os.WriteFile(notDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, static := range []string{
		filepath.Join(tmp, "missing"),
		notDir,
	} {
		err := app.Run(context.Background(), app.Config{
			ConfigPath: testdataPath(t, "valid.yaml"),
			StaticDir:  static,
		})
		if err == nil || !strings.Contains(err.Error(), "register static files") {
			t.Fatalf("static=%q: err=%v, want containing %q", static, err, "register static files")
		}
	}
}

func TestMountStatic(t *testing.T) {
	tmp := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmp, "test.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	notDir := filepath.Join(tmp, "file")
	if err := os.WriteFile(notDir, []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tests := []struct {
		name         string
		staticDir    string
		wantMountErr bool
		wantBody     string
	}{
		{
			name:      "override dir serves files",
			staticDir: tmp,
			wantBody:  "ok",
		},
		{
			name:         "missing dir errors",
			staticDir:    filepath.Join(tmp, "missing"),
			wantMountErr: true,
		},
		{
			name:         "non-directory path errors",
			staticDir:    notDir,
			wantMountErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			err := app.MountStatic(e, tt.staticDir)
			if tt.wantMountErr {
				if err == nil {
					t.Fatal("MountStatic: expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("MountStatic: unexpected error: %v", err)
			}

			if tt.wantBody != "" {
				req := httptest.NewRequest(http.MethodGet, "/test.txt", nil)
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					t.Fatalf("status: want 200, got %d", rec.Code)
				}
				if got := rec.Body.String(); got != tt.wantBody {
					t.Fatalf("body: want %q, got %q", tt.wantBody, got)
				}
			}
		})
	}
}

func TestRun_StartsAndShutsDown(t *testing.T) {
	addrCh := make(chan net.Addr, 1)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(ctx, app.Config{
			ConfigPath: testdataPath(t, "valid.yaml"),
		}, app.WithListenerAddrFunc(func(a net.Addr) {
			addrCh <- a
		}))
	}()

	var addr net.Addr
	select {
	case addr = <-addrCh:
	case <-time.After(2 * time.Second):
		t.Fatal("listener address not reported")
	}

	conn, err := net.DialTimeout("tcp", addr.String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	_ = conn.Close()

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop")
	}
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func newEcho(mw ...echo.MiddlewareFunc) *echo.Echo {
	e := echo.New()
	for _, m := range mw {
		e.Use(m)
	}
	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/favicon.svg", func(c *echo.Context) error {
		return c.String(http.StatusOK, "")
	})
	e.GET("/other", func(c *echo.Context) error {
		return c.String(http.StatusOK, "")
	})
	return e
}

func doGet(e *echo.Echo, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestSecurityMiddleware(t *testing.T) {
	csp := "default-src 'self'"
	rec := doGet(newEcho(SecurityMiddleware(csp)), "/test")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != csp {
		t.Errorf("Content-Security-Policy = %q, want %q", got, csp)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want %q", got, "DENY")
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Errorf("Referrer-Policy = %q, want %q", got, "strict-origin-when-cross-origin")
	}
}

func TestFaviconCORS(t *testing.T) {
	e := newEcho(FaviconCORS())

	tests := []struct {
		name     string
		path     string
		wantACAO string
		wantCORP string
	}{
		{name: "favicon_sets_cors", path: "/favicon.svg", wantACAO: "*", wantCORP: "cross-origin"},
		{name: "other_skips_cors", path: "/other", wantACAO: "", wantCORP: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doGet(e, tt.path)
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tt.wantACAO {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tt.wantACAO)
			}
			if got := rec.Header().Get("Cross-Origin-Resource-Policy"); got != tt.wantCORP {
				t.Errorf("Cross-Origin-Resource-Policy = %q, want %q", got, tt.wantCORP)
			}
		})
	}
}

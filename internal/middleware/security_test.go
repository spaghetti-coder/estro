package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func testEcho() *echo.Echo {
	e := echo.New()
	return e
}

func TestSecurityMiddlewareSetsHeaders(t *testing.T) {
	e := testEcho()
	csp := "default-src 'self'"
	e.Use(SecurityMiddleware(csp))
	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	tests := []struct {
		header string
		want   string
	}{
		{"Content-Security-Policy", csp},
		{"X-Frame-Options", "DENY"},
		{"X-Content-Type-Options", "nosniff"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
	}
	for _, tt := range tests {
		got := rec.Header().Get(tt.header)
		if got != tt.want {
			t.Errorf("header %q = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestSecurityMiddlewareAllowsRequest(t *testing.T) {
	e := testEcho()
	e.Use(SecurityMiddleware("default-src 'self'"))
	e.GET("/test", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestFaviconCORSSetsHeaders(t *testing.T) {
	e := testEcho()
	e.Use(FaviconCORS())
	e.GET("/favicon.svg", func(c *echo.Context) error {
		return c.String(http.StatusOK, "")
	})
	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
	if got := rec.Header().Get("Cross-Origin-Resource-Policy"); got != "cross-origin" {
		t.Errorf("Cross-Origin-Resource-Policy = %q, want %q", got, "cross-origin")
	}
}

func TestFaviconCORSNoOtherRoutes(t *testing.T) {
	e := testEcho()
	e.Use(FaviconCORS())
	e.GET("/other", func(c *echo.Context) error {
		return c.String(http.StatusOK, "")
	})
	req := httptest.NewRequest(http.MethodGet, "/other", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin on non-favicon route = %q, want empty", got)
	}
}
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
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	tests := []struct {
		name      string
		header    string
		wantValue string
	}{
		{name: "Content-Security-Policy", header: "Content-Security-Policy", wantValue: csp},
		{name: "X-Frame-Options", header: "X-Frame-Options", wantValue: "DENY"},
		{name: "X-Content-Type-Options", header: "X-Content-Type-Options", wantValue: "nosniff"},
		{name: "Referrer-Policy", header: "Referrer-Policy", wantValue: "strict-origin-when-cross-origin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rec.Header().Get(tt.header); got != tt.wantValue {
				t.Errorf("%s = %q, want %q", tt.header, got, tt.wantValue)
			}
		})
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
		{name: "favicon sets cors", path: "/favicon.svg", wantACAO: "*", wantCORP: "cross-origin"},
		{name: "other skips cors", path: "/other", wantACAO: "", wantCORP: ""},
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

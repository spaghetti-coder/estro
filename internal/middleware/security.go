package middleware

import (
	"github.com/labstack/echo/v5"
)

func SecurityMiddleware(csp string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Set("Content-Security-Policy", csp)
			c.Response().Header().Set("X-Frame-Options", "DENY")
			c.Response().Header().Set("X-Content-Type-Options", "nosniff")
			c.Response().Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			return next(c)
		}
	}
}

func FaviconCORS() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if c.Request().URL.Path == "/favicon.svg" {
				c.Response().Header().Set("Access-Control-Allow-Origin", "*")
				c.Response().Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
			}
			return next(c)
		}
	}
}

// Package auth provides session-based authentication and password comparison for Estro.
package auth

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo/v5"
	"github.com/spaghetti-coder/estro/internal/config"
	"golang.org/x/crypto/bcrypt"
)

// SessionCookieName is the cookie name used for session storage.
const SessionCookieName = "connect.sid"

const rememberMeMaxAge = 30 * 24 * 3600

// Authenticate verifies a username/password combination against the
// provided user map. Returns nil on failure.
func Authenticate(users map[string]*config.UserConfig, username, password string) *config.UserConfig {
	user, ok := users[username]
	if !ok {
		return nil
	}
	if !ComparePassword(user.Password, password) {
		return nil
	}
	return user
}

// HashPassword generates a bcrypt hash from a plain-text password
// and returns it with the "bcrypt:" prefix used by ComparePassword.
func HashPassword(plainPassword string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), 13)
	if err != nil {
		return "", fmt.Errorf("generate bcrypt hash: %w", err)
	}
	return "bcrypt:" + string(hash), nil
}

// ComparePassword compares a stored password with a plain-text password.
// The stored password can be prefixed with "plain:" for plaintext comparison
// or "bcrypt:" for bcrypt comparison. Unprefixed values are treated as bcrypt
// hashes for backward compatibility.
func ComparePassword(stored, plainPassword string) bool {
	if strings.HasPrefix(stored, "plain:") {
		return stored[len("plain:"):] == plainPassword
	}
	hash := strings.TrimPrefix(stored, "bcrypt:")
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plainPassword)) == nil
}

// GetSessionUser retrieves the authenticated username from the session cookie.
// Returns an empty string if no user is logged in.
func GetSessionUser(store sessions.Store, r *http.Request, w http.ResponseWriter) (string, error) {
	// Error ignored: Get() returns a valid session even on decode errors.
	session, _ := store.Get(r, SessionCookieName)
	username, ok := session.Values["user"].(string)
	if !ok || username == "" {
		return "", nil
	}
	return username, nil
}

// SetSessionUser stores the username in the session cookie.
// When rememberMe is true, the session persists for 30 days.
func SetSessionUser(store sessions.Store, r *http.Request, w http.ResponseWriter, username string, rememberMe bool) error {
	session, _ := store.Get(r, SessionCookieName)
	session.Values["user"] = username
	maxAge := 0
	if rememberMe {
		maxAge = rememberMeMaxAge
	}
	session.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	return session.Save(r, w)
}

// GenerateSessionSecret creates a cryptographically random 32-byte secret
// suitable for use as a session cookie signing key.
func GenerateSessionSecret() ([]byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate session secret: %w", err)
	}
	return b, nil
}

// NewSessionStore creates a new cookie-based session store signed with
// the provided secret.
func NewSessionStore(secret []byte) sessions.Store {
	return sessions.NewCookieStore(secret)
}

// SessionMiddleware injects the session store into the echo context so
// that handlers can retrieve it via c.Get("_session_store").
func SessionMiddleware(store sessions.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("_session_store", store)
			return next(c)
		}
	}
}

// DestroySession clears the session cookie and expires it immediately.
func DestroySession(store sessions.Store, r *http.Request, w http.ResponseWriter) error {
	// Error ignored: Get() always returns a usable session.
	session, _ := store.Get(r, SessionCookieName)
	session.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	session.Values = map[interface{}]interface{}{}
	return session.Save(r, w)
}

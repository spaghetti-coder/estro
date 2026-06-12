// Package auth provides session-based authentication and password comparison for Estro.
package auth

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
	"github.com/spaghetti-coder/estro/internal/config"
	"golang.org/x/crypto/bcrypt"
)

const SessionCookieName = "connect.sid"

const rememberMeMaxAge = 30 * 24 * 3600

var bcryptCost = 13

// SetBcryptCost overrides bcrypt cost (tests only).
func SetBcryptCost(c int) { bcryptCost = c }

// Authenticate checks username/password; returns user or nil.
func Authenticate(users map[string]*config.UserConfig, username, password string) *config.UserConfig {
	user, ok := users[username]
	if !ok {
		return nil
	}

	stored := user.Password
	if strings.HasPrefix(stored, "plain:") {
		if strings.TrimPrefix(stored, "plain:") == password {
			return user
		}
		return nil
	}

	hash := strings.TrimPrefix(stored, "bcrypt:")
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil {
		return user
	}
	return nil
}

// HashPassword returns a bcrypt hash with "bcrypt:" prefix.
func HashPassword(plainPassword string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("generate bcrypt hash: %w", err)
	}
	return "bcrypt:" + string(hash), nil
}

// GetSessionUser returns the logged-in username, or "".
func GetSessionUser(store sessions.Store, r *http.Request) string {
	session, _ := store.Get(r, SessionCookieName) // Get always returns a usable session.
	username, _ := session.Values["user"].(string)
	return username
}

// SetSessionUser saves username to session; 30-day max-age when rememberMe.
func SetSessionUser(store sessions.Store, r *http.Request, w http.ResponseWriter, username string, rememberMe bool) error {
	session, _ := store.Get(r, SessionCookieName)
	session.Values["user"] = username
	session.Options = getSessionOptions(0)
	if rememberMe {
		session.Options.MaxAge = rememberMeMaxAge
	}
	return session.Save(r, w)
}

// GenerateSessionSecret returns a random 32-byte signing key.
func GenerateSessionSecret() ([]byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate session secret: %w", err)
	}
	return b, nil
}

// NewSessionStore returns a cookie store signed with secret.
func NewSessionStore(secret []byte) sessions.Store {
	return sessions.NewCookieStore(secret)
}

// DestroySession expires and clears the session.
func DestroySession(store sessions.Store, r *http.Request, w http.ResponseWriter) error {
	session, _ := store.Get(r, SessionCookieName) // Get always returns a usable session.
	session.Options = getSessionOptions(-1)
	session.Values = map[any]any{}
	return session.Save(r, w)
}

func getSessionOptions(maxAge int) *sessions.Options {
	return &sessions.Options{
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
}

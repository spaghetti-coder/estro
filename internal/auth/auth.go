// Package auth provides session-based authentication and password comparison for Estro.
package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/sessions"
	"github.com/spaghetti-coder/estro/internal/config"
	"golang.org/x/crypto/bcrypt"
)

const SessionCookieName = "connect.sid"

var bcryptCost = 13

// ErrSessionExpired is returned when a session's absolute expiry has passed.
var ErrSessionExpired = errors.New("session expired")

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

// HashPassword returns a bcrypt hash.
func HashPassword(plainPassword string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("generate bcrypt hash: %w", err)
	}
	return string(hash), nil
}

// GetSessionUser returns the logged-in username, or "".
func GetSessionUser(store sessions.Store, r *http.Request) string {
	session, _ := store.Get(r, SessionCookieName) // Get always returns a usable session.
	username, _ := session.Values["user"].(string)
	return username
}

// SetSessionUser saves username to session. Stores remember_me and expires_at for finite remember-me TTLs.
func SetSessionUser(store sessions.Store, r *http.Request, w http.ResponseWriter, username string, rememberMe bool, maxAge int) error {
	session, _ := store.Get(r, SessionCookieName)
	session.Values["user"] = username
	session.Options = getSessionOptions(0)
	if rememberMe {
		session.Options.MaxAge = maxAge
		session.Values["remember_me"] = true
		if maxAge > 0 && maxAge != math.MaxInt32 {
			session.Values["expires_at"] = time.Now().Add(time.Duration(maxAge) * time.Second).Unix()
		} else {
			delete(session.Values, "expires_at")
		}
	} else {
		delete(session.Values, "remember_me")
		delete(session.Values, "expires_at")
	}
	return session.Save(r, w)
}

// RefreshSession slides cookie for remember-me sessions. Caps MaxAge at remaining time
// via expires_at. Returns ErrSessionExpired past deadline. No-op for session-only cookies.
func RefreshSession(store sessions.Store, r *http.Request, w http.ResponseWriter, configuredTTL int) error {
	session, _ := store.Get(r, SessionCookieName)
	if _, ok := session.Values["user"].(string); !ok {
		return nil // no session to refresh
	}

	rememberMe, _ := session.Values["remember_me"].(bool)
	if !rememberMe {
		return nil // session-only cookie, don't convert to persistent
	}

	// Check absolute expiry (source of truth)
	if exp, ok := session.Values["expires_at"].(int64); ok {
		if time.Now().Unix() > exp {
			return ErrSessionExpired
		}
		remaining := int(time.Until(time.Unix(exp, 0)).Seconds())
		if remaining <= 0 {
			return ErrSessionExpired
		}
		session.Options = getSessionOptions(remaining)
	} else {
		// No absolute expiry (no-limit session)
		session.Options = getSessionOptions(configuredTTL)
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

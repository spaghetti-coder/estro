package auth

import (
	"bytes"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/sessions"
	"github.com/spaghetti-coder/estro/internal/config"
	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	SetBcryptCost(bcrypt.MinCost)
	os.Exit(m.Run())
}

// setSession creates a request carrying a session cookie for username.
func setSession(t *testing.T, store sessions.Store, username string, rememberMe bool, maxAge int) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	if err := SetSessionUser(store, req, rec, username, rememberMe, maxAge); err != nil {
		t.Fatalf("SetSessionUser: %v", err)
	}
	return followRequest(rec)
}

// followRequest returns a fresh request carrying the cookies from rec.
func followRequest(rec *httptest.ResponseRecorder) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

func TestGetSessionUser(t *testing.T) {
	store := NewSessionStore([]byte("test-secret-get"))
	t.Run("unauthenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if got := GetSessionUser(store, req); got != "" {
			t.Errorf("expected empty username, got %q", got)
		}
	})
	t.Run("authenticated", func(t *testing.T) {
		req := setSession(t, store, "alice", false, 0)
		if got := GetSessionUser(store, req); got != "alice" {
			t.Errorf("expected username %q, got %q", "alice", got)
		}
	})
}

func TestSetSessionUser(t *testing.T) {
	tests := []struct {
		name          string
		rememberMe    bool
		maxAge        int
		wantRemember  bool
		wantExpiresAt bool
		checkExpires  bool
	}{
		{name: "session-only", rememberMe: false, maxAge: 0, wantRemember: false, wantExpiresAt: false},
		{name: "remember finite TTL", rememberMe: true, maxAge: 86400, wantRemember: true, wantExpiresAt: true, checkExpires: true},
		{name: "remember no-limit", rememberMe: true, maxAge: math.MaxInt32, wantRemember: true, wantExpiresAt: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewSessionStore([]byte("test-secret-" + tt.name))
			req := setSession(t, store, "alice", tt.rememberMe, tt.maxAge)

			if got := GetSessionUser(store, req); got != "alice" {
				t.Errorf("expected user 'alice', got %q", got)
			}

			session, _ := store.Get(req, SessionCookieName)
			if _, ok := session.Values["remember_me"].(bool); ok != tt.wantRemember {
				t.Errorf("remember_me present = %v, want %v", ok, tt.wantRemember)
			}
			expiresAt, ok := session.Values["expires_at"].(int64)
			if ok != tt.wantExpiresAt {
				t.Errorf("expires_at present = %v, want %v", ok, tt.wantExpiresAt)
			}
			if tt.checkExpires {
				expected := time.Now().Add(time.Duration(tt.maxAge) * time.Second).Truncate(time.Second)
				got := time.Unix(expiresAt, 0)
				if delta := got.Sub(expected); delta < -2*time.Second || delta > time.Second {
					t.Errorf("expires_at = %v, want ~%v (delta %v)", got, expected, delta)
				}
			}
		})
	}

	t.Run("clears previous remember flags", func(t *testing.T) {
		store := NewSessionStore([]byte("test-secret-clear-flags"))

		// First rememberMe=true login
		req := setSession(t, store, "alice", true, 86400)

		// Then re-login with rememberMe=false (e.g., different device)
		rec2 := httptest.NewRecorder()
		if err := SetSessionUser(store, req, rec2, "alice", false, 0); err != nil {
			t.Fatalf("SetSessionUser: %v", err)
		}

		// Verify remember_me and expires_at are cleared
		req2 := followRequest(rec2)
		session, _ := store.Get(req2, SessionCookieName)

		if _, ok := session.Values["remember_me"]; ok {
			t.Error("expected remember_me to be cleared")
		}
		if _, ok := session.Values["expires_at"]; ok {
			t.Error("expected expires_at to be cleared")
		}
	})
}

func TestAuthenticatePasswordFormats(t *testing.T) {
	bcryptHash, err := bcrypt.GenerateFromPassword([]byte("secretpass"), bcryptCost)
	if err != nil {
		t.Fatalf("failed to generate hash: %v", err)
	}

	tests := []struct {
		name   string
		stored string
		plain  string
		want   bool
	}{
		{name: "bare bcrypt hash correct password", stored: string(bcryptHash), plain: "secretpass", want: true},
		{name: "bcrypt prefix correct password", stored: "bcrypt:" + string(bcryptHash), plain: "secretpass", want: true},
		{name: "bcrypt prefix wrong password", stored: "bcrypt:" + string(bcryptHash), plain: "wrongpass", want: false},
		{name: "plain prefix correct password", stored: "plain:secretpass", plain: "secretpass", want: true},
		{name: "plain prefix wrong password", stored: "plain:secretpass", plain: "wrongpass", want: false},
		{name: "plain prefix empty password matches empty", stored: "plain:", plain: "", want: true},
		{name: "plain prefix empty password does not match nonempty", stored: "plain:", plain: "notempty", want: false},
		{name: "empty stored hash", stored: "", plain: "x", want: false},
		{name: "invalid bcrypt hash", stored: "not-a-hash", plain: "x", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := map[string]*config.UserConfig{"u": {Password: tt.stored}}
			got := Authenticate(users, "u", tt.plain)
			if (got != nil) != tt.want {
				t.Errorf("Authenticate(..., u, %q) = %v, want non-nil %v", tt.plain, got, tt.want)
			}
		})
	}
}

func TestAuthenticateUnknownUser(t *testing.T) {
	// nil and empty maps both miss on lookup (no panic); exercise both defensively.
	for _, users := range []map[string]*config.UserConfig{nil, {}} {
		if got := Authenticate(users, "ghost", "x"); got != nil {
			t.Errorf("Authenticate(%v, ghost, x) = %v, want nil", users, got)
		}
	}
}

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name    string
		plain   string
		wantErr bool
	}{
		{name: "simple password", plain: "testpass"},
		{name: "empty password", plain: ""},
		{name: "password with special chars", plain: "p@$$w0rd!#"},
		{name: "password exceeding bcrypt limit", plain: strings.Repeat("a", 73), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.plain)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("HashPassword(%q) expected error, got nil", tt.plain)
				}
				if !strings.Contains(err.Error(), "generate bcrypt hash") {
					t.Errorf("expected wrapped error containing %q, got %q", "generate bcrypt hash", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("HashPassword(%q) returned error: %v", tt.plain, err)
			}
			if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(tt.plain)); err != nil {
				t.Errorf("bcrypt hash does not match plain password %q: %v", tt.plain, err)
			}
		})
	}
}

func TestRefreshSession(t *testing.T) {
	t.Run("refreshes remember-me session", func(t *testing.T) {
		tests := []struct {
			name   string
			user   string
			maxAge int
			hasExp bool
		}{
			{name: "no-limit", user: "alice", maxAge: math.MaxInt32, hasExp: false},
			{name: "finite TTL", user: "carol", maxAge: 86400, hasExp: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				s := NewSessionStore([]byte("test-refresh-" + tt.name))

				// Set
				req := setSession(t, s, tt.user, true, tt.maxAge)

				// Refresh
				rec := httptest.NewRecorder()
				if err := RefreshSession(s, req, rec, tt.maxAge); err != nil {
					t.Fatalf("RefreshSession: %v", err)
				}

				// Verify user survives
				req2 := followRequest(rec)
				if got := GetSessionUser(s, req2); got != tt.user {
					t.Errorf("user = %q, want %q", got, tt.user)
				}

				// Verify remember_me persists
				session, _ := s.Get(req2, SessionCookieName)
				if rm, _ := session.Values["remember_me"].(bool); !rm {
					t.Error("remember_me lost after refresh")
				}

				// Verify expires_at presence matches expectation
				_, hasExpiresAt := session.Values["expires_at"]
				if hasExpiresAt != tt.hasExp {
					t.Errorf("expires_at present = %v, want %v", hasExpiresAt, tt.hasExp)
				}
			})
		}
	})

	t.Run("no-op without session", func(t *testing.T) {
		freshStore := NewSessionStore([]byte("test-secret2"))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		if err := RefreshSession(freshStore, req, rec, math.MaxInt32); err != nil {
			t.Fatalf("RefreshSession on no session error: %v", err)
		}
		if got := GetSessionUser(freshStore, req); got != "" {
			t.Errorf("expected empty username, got %q", got)
		}
	})

	t.Run("no-op for session-only cookie (rememberMe=false)", func(t *testing.T) {
		s := NewSessionStore([]byte("test-secret3"))
		req := setSession(t, s, "bob", false, 0)
		rec2 := httptest.NewRecorder()
		if err := RefreshSession(s, req, rec2, math.MaxInt32); err != nil {
			t.Fatalf("RefreshSession error: %v", err)
		}
		if hasCookie := rec2.Header().Get("Set-Cookie"); hasCookie != "" {
			t.Error("session-only cookie should not be refreshed to persistent")
		}
	})

	t.Run("returns ErrSessionExpired when expires_at is in the past", func(t *testing.T) {
		s := NewSessionStore([]byte("test-secret4"))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		session, _ := s.Get(req, SessionCookieName)
		session.Values["user"] = "alice"
		session.Values["remember_me"] = true
		session.Values["expires_at"] = time.Now().Add(-time.Hour).Unix() // expired 1 hour ago
		session.Options = getSessionOptions(3600)
		if err := session.Save(req, rec); err != nil {
			t.Fatalf("session save error: %v", err)
		}

		req2 := followRequest(rec)
		rec2 := httptest.NewRecorder()
		err := RefreshSession(s, req2, rec2, 3600)
		if err != ErrSessionExpired {
			t.Errorf("expected ErrSessionExpired, got %v", err)
		}
	})

	t.Run("caps MaxAge at remaining time near expiry", func(t *testing.T) {
		s := NewSessionStore([]byte("test-secret6"))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		// Session expires in 5 minutes, but configured TTL is 30 days
		session, _ := s.Get(req, SessionCookieName)
		session.Values["user"] = "dave"
		session.Values["remember_me"] = true
		session.Values["expires_at"] = time.Now().Add(5 * time.Minute).Unix()
		session.Options = getSessionOptions(86400)
		if err := session.Save(req, rec); err != nil {
			t.Fatalf("session save error: %v", err)
		}

		req2 := followRequest(rec)
		rec2 := httptest.NewRecorder()
		if err := RefreshSession(s, req2, rec2, 2592000); err != nil {
			t.Fatalf("RefreshSession error: %v", err)
		}

		// Verify session valid after refresh with capped MaxAge
		req3 := followRequest(rec2)
		if got := GetSessionUser(s, req3); got != "dave" {
			t.Errorf("expected dave after refresh, got %q", got)
		}

		// Verify Set-Cookie has a small Max-Age (near 300s), not 2592000s (30 days)
		setCookie := rec2.Header().Get("Set-Cookie")
		if setCookie == "" {
			t.Fatal("expected Set-Cookie header after refresh")
		}
		// Max-Age should be ~300, not 2592000
		_, ageStr, ok := strings.Cut(setCookie, "Max-Age=")
		if !ok {
			t.Fatal("expected Max-Age in Set-Cookie header")
		}
		if semi := strings.IndexByte(ageStr, ';'); semi != -1 {
			ageStr = ageStr[:semi]
		}
		maxAge, err := strconv.Atoi(ageStr)
		if err != nil {
			t.Fatalf("failed to parse Max-Age %q: %v", ageStr, err)
		}
		if maxAge < 290 || maxAge > 310 {
			t.Errorf("expected MaxAge near 300s (5min), got %ds", maxAge)
		}
	})

	t.Run("remaining zero", func(t *testing.T) {
		s := NewSessionStore([]byte("test-zero"))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		session, _ := s.Get(req, SessionCookieName)
		session.Values["user"] = "x"
		session.Values["remember_me"] = true
		session.Values["expires_at"] = time.Now().Unix()
		if err := session.Save(req, rec); err != nil {
			t.Fatalf("session save: %v", err)
		}

		err := RefreshSession(s, followRequest(rec), httptest.NewRecorder(), 3600)
		if err != ErrSessionExpired {
			t.Errorf("want ErrSessionExpired, got %v", err)
		}
	})
}

func TestGenerateSessionSecret(t *testing.T) {
	secret, err := GenerateSessionSecret()
	if err != nil {
		t.Fatalf("GenerateSessionSecret: %v", err)
	}
	if len(secret) != 32 {
		t.Errorf("expected 32-byte secret, got %d bytes", len(secret))
	}
	// Two calls should produce different secrets.
	secret2, _ := GenerateSessionSecret()
	if bytes.Equal(secret, secret2) {
		t.Error("two consecutive calls produced identical secrets")
	}
}

func TestDestroySession(t *testing.T) {
	store := NewSessionStore([]byte("test-secret-destroy"))
	req := setSession(t, store, "alice", false, 0)
	rec2 := httptest.NewRecorder()
	if err := DestroySession(store, req, rec2); err != nil {
		t.Fatalf("failed to destroy session: %v", err)
	}

	setCookie := rec2.Header().Get("Set-Cookie")
	if setCookie == "" {
		t.Fatal("expected Set-Cookie on destroy")
	}
	parsed, err := http.ParseSetCookie(setCookie)
	if err != nil {
		t.Fatalf("ParseSetCookie: %v", err)
	}
	if parsed.MaxAge != -1 {
		t.Errorf("expected MaxAge=-1 on destroy, got %d", parsed.MaxAge)
	}

	req2 := followRequest(rec2)
	username := GetSessionUser(store, req2)
	if username != "" {
		t.Errorf("expected empty username after session destruction, got %q", username)
	}
}

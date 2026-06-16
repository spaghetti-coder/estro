package auth

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/sessions"
	"github.com/spaghetti-coder/estro/internal/config"
	"golang.org/x/crypto/bcrypt"
)

func init() {
	SetBcryptCost(bcrypt.MinCost)
}

// setSession records a SetSessionUser call and returns a request
// carrying the resulting session cookie.
func setSession(t *testing.T, store sessions.Store, username string, rememberMe bool, maxAge int) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	if err := SetSessionUser(store, req, rec, username, rememberMe, maxAge); err != nil {
		t.Fatalf("SetSessionUser: %v", err)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req2.AddCookie(c)
	}
	return req2
}

func TestGetSessionUserNotAuthenticated(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret-not-auth"))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	username := GetSessionUser(store, req)
	if username != "" {
		t.Errorf("expected empty username for unauthenticated session, got %q", username)
	}
}

func TestSetAndGetSessionUser(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret-set-get"))
	req := setSession(t, store, "alice", false, 0)
	if username := GetSessionUser(store, req); username != "alice" {
		t.Errorf("expected username %q, got %q", "alice", username)
	}
}

func TestSetSessionUserRememberMeWithTTL(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret-ttl"))
	ttlSeconds := 86400 // 1 day
	req := setSession(t, store, "alice", true, ttlSeconds)
	session, _ := store.Get(req, SessionCookieName)

	if got, _ := session.Values["user"].(string); got != "alice" {
		t.Errorf("expected user 'alice', got %q", got)
	}
	if got, _ := session.Values["remember_me"].(bool); !got {
		t.Error("expected remember_me=true")
	}
	expiresAt, ok := session.Values["expires_at"].(int64)
	if !ok {
		t.Fatal("expected expires_at to be set")
	}
	expectedExpiry := time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	if time.Unix(expiresAt, 0).Before(time.Now().Add(time.Duration(ttlSeconds-5) * time.Second)) {
		t.Errorf("expires_at too early: %v", time.Unix(expiresAt, 0))
	}
	if time.Unix(expiresAt, 0).After(expectedExpiry.Add(5 * time.Second)) {
		t.Errorf("expires_at too late: %v", time.Unix(expiresAt, 0))
	}
}

func TestSetSessionUserRememberMeNoLimit(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret-no-limit"))
	req := setSession(t, store, "alice", true, math.MaxInt32)
	session, _ := store.Get(req, SessionCookieName)

	if got, _ := session.Values["remember_me"].(bool); !got {
		t.Error("expected remember_me=true")
	}
	if _, ok := session.Values["expires_at"]; ok {
		t.Error("expected no expires_at for no-limit session")
	}
}

func TestSetSessionUserNoRememberMeClearsFlags(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret-clear-flags"))

	// First login with rememberMe=true
	req := setSession(t, store, "alice", true, 86400)

	// Then re-login with rememberMe=false (e.g., different device)
	rec2 := httptest.NewRecorder()
	if err := SetSessionUser(store, req, rec2, "alice", false, 0); err != nil {
		t.Fatalf("SetSessionUser: %v", err)
	}

	// Verify remember_me and expires_at are cleared
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec2.Result().Cookies() {
		req2.AddCookie(c)
	}
	session, _ := store.Get(req2, SessionCookieName)

	if _, ok := session.Values["remember_me"]; ok {
		t.Error("expected remember_me to be cleared")
	}
	if _, ok := session.Values["expires_at"]; ok {
		t.Error("expected expires_at to be cleared")
	}
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
		{
			name:   "bare bcrypt hash correct password",
			stored: string(bcryptHash),
			plain:  "secretpass",
			want:   true,
		},
		{
			name:   "bare bcrypt hash wrong password",
			stored: string(bcryptHash),
			plain:  "wrongpass",
			want:   false,
		},
		{
			name:   "bcrypt prefix correct password",
			stored: "bcrypt:" + string(bcryptHash),
			plain:  "secretpass",
			want:   true,
		},
		{
			name:   "bcrypt prefix wrong password",
			stored: "bcrypt:" + string(bcryptHash),
			plain:  "wrongpass",
			want:   false,
		},
		{
			name:   "plain prefix correct password",
			stored: "plain:secretpass",
			plain:  "secretpass",
			want:   true,
		},
		{
			name:   "plain prefix wrong password",
			stored: "plain:secretpass",
			plain:  "wrongpass",
			want:   false,
		},
		{
			name:   "plain prefix empty password matches empty",
			stored: "plain:",
			plain:  "",
			want:   true,
		},
		{
			name:   "plain prefix empty password does not match nonempty",
			stored: "plain:",
			plain:  "notempty",
			want:   false,
		},
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

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name  string
		plain string
	}{
		{name: "simple password", plain: "testpass"},
		{name: "empty password", plain: ""},
		{name: "long password", plain: strings.Repeat("a", 72)},
		{name: "password with special chars", plain: "p@$$w0rd!#"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.plain)
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
				s := sessions.NewCookieStore([]byte("test-refresh-" + tt.name))

				// Set
				req := setSession(t, s, tt.user, true, tt.maxAge)

				// Refresh
				rec := httptest.NewRecorder()
				if err := RefreshSession(s, req, rec, tt.maxAge); err != nil {
					t.Fatalf("RefreshSession: %v", err)
				}

				// Verify user survives
				req2 := httptest.NewRequest(http.MethodGet, "/", nil)
				for _, c := range rec.Result().Cookies() {
					req2.AddCookie(c)
				}
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
		freshStore := sessions.NewCookieStore([]byte("test-secret2"))
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
		s := sessions.NewCookieStore([]byte("test-secret3"))
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
		s := sessions.NewCookieStore([]byte("test-secret4"))
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

		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		for _, c := range rec.Result().Cookies() {
			req2.AddCookie(c)
		}
		rec2 := httptest.NewRecorder()
		err := RefreshSession(s, req2, rec2, 3600)
		if err != ErrSessionExpired {
			t.Errorf("expected ErrSessionExpired, got %v", err)
		}
	})

	t.Run("caps MaxAge at remaining time near expiry", func(t *testing.T) {
		s := sessions.NewCookieStore([]byte("test-secret6"))
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

		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		for _, c := range rec.Result().Cookies() {
			req2.AddCookie(c)
		}
		rec2 := httptest.NewRecorder()
		if err := RefreshSession(s, req2, rec2, 2592000); err != nil {
			t.Fatalf("RefreshSession error: %v", err)
		}

		// Verify session remains valid after refresh with capped MaxAge
		req3 := httptest.NewRequest(http.MethodGet, "/", nil)
		for _, c := range rec2.Result().Cookies() {
			req3.AddCookie(c)
		}
		if got := GetSessionUser(s, req3); got != "dave" {
			t.Errorf("expected dave after refresh, got %q", got)
		}

		// Verify Set-Cookie has a small Max-Age (near 300s), not 2592000s (30 days)
		setCookie := rec2.Header().Get("Set-Cookie")
		if setCookie == "" {
			t.Fatal("expected Set-Cookie header after refresh")
		}
		// Max-Age should be ~300, not 2592000
		idx := strings.Index(setCookie, "Max-Age=")
		if idx == -1 {
			t.Fatal("expected Max-Age in Set-Cookie header")
		}
		ageStr := setCookie[idx+len("Max-Age="):]
		if semi := strings.IndexByte(ageStr, ';'); semi != -1 {
			ageStr = ageStr[:semi]
		}
		maxAge, err := strconv.Atoi(ageStr)
		if err != nil {
			t.Fatalf("failed to parse Max-Age %q: %v", ageStr, err)
		}
		if maxAge <= 0 || maxAge > 600 {
			t.Errorf("expected MaxAge near 300s (5min), got %ds", maxAge)
		}
	})
}

func TestDestroySession(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret-destroy"))
	req := setSession(t, store, "alice", false, 0)
	rec2 := httptest.NewRecorder()
	if err := DestroySession(store, req, rec2); err != nil {
		t.Fatalf("failed to destroy session: %v", err)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec2.Result().Cookies() {
		req2.AddCookie(c)
	}
	username := GetSessionUser(store, req2)
	if username != "" {
		t.Errorf("expected empty username after session destruction, got %q", username)
	}
}

package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
)

func TestComparePasswordCorrect(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("testpass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to generate hash: %v", err)
	}
	if err := ComparePassword(string(hash), "testpass"); err != nil {
		t.Errorf("expected nil error for correct password, got: %v", err)
	}
}

func TestComparePasswordWrong(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("testpass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to generate hash: %v", err)
	}
	if err := ComparePassword(string(hash), "wrongpass"); err == nil {
		t.Error("expected error for wrong password, got nil")
	}
}

func TestComparePasswordRealConfig(t *testing.T) {
	if err := ComparePassword("$2y$10$6c9tQEpF4w2Ev9XCLH2pauAawy2874wwgN5jCTyrYMYclVlTTNIs2", "changeme1"); err != nil {
		t.Errorf("expected alice's password to match, got: %v", err)
	}
}

func TestGetSessionUserNotAuthenticated(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	username, err := GetSessionUser(store, req, rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if username != "" {
		t.Errorf("expected empty username for unauthenticated session, got %q", username)
	}
}

func TestSetAndGetSessionUser(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	if err := SetSessionUser(store, req, rec, "alice", false); err != nil {
		t.Fatalf("failed to set session user: %v", err)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Cookie", rec.Header().Get("Set-Cookie"))
	rec2 := httptest.NewRecorder()
	username, err := GetSessionUser(store, req2, rec2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if username != "alice" {
		t.Errorf("expected username %q, got %q", "alice", username)
	}
}

func TestSetSessionUserRememberMe(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	if err := SetSessionUser(store, req, rec, "bob", true); err != nil {
		t.Fatalf("failed to set session user: %v", err)
	}
	cookie := rec.Header().Get("Set-Cookie")
	if cookie == "" {
		t.Fatal("expected Set-Cookie header")
	}
	if !strings.Contains(cookie, "Max-Age=") && !strings.Contains(cookie, "Expires=") {
		t.Error("expected persistent cookie with Max-Age for rememberMe=true")
	}
}

func TestComparePasswordFormats(t *testing.T) {
	bcryptHash, err := bcrypt.GenerateFromPassword([]byte("secretpass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to generate hash: %v", err)
	}

	tests := []struct {
		name    string
		stored  string
		plain   string
		wantErr bool
	}{
		{
			name:    "bare bcrypt hash correct password",
			stored:  string(bcryptHash),
			plain:   "secretpass",
			wantErr: false,
		},
		{
			name:    "bare bcrypt hash wrong password",
			stored:  string(bcryptHash),
			plain:   "wrongpass",
			wantErr: true,
		},
		{
			name:    "bcrypt prefix correct password",
			stored:  "bcrypt:" + string(bcryptHash),
			plain:   "secretpass",
			wantErr: false,
		},
		{
			name:    "bcrypt prefix wrong password",
			stored:  "bcrypt:" + string(bcryptHash),
			plain:   "wrongpass",
			wantErr: true,
		},
		{
			name:    "plain prefix correct password",
			stored:  "plain:secretpass",
			plain:   "secretpass",
			wantErr: false,
		},
		{
			name:    "plain prefix wrong password",
			stored:  "plain:secretpass",
			plain:   "wrongpass",
			wantErr: true,
		},
		{
			name:    "plain prefix empty password matches empty",
			stored:  "plain:",
			plain:   "",
			wantErr: false,
		},
		{
			name:    "plain prefix empty password does not match nonempty",
			stored:  "plain:",
			plain:   "notempty",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ComparePassword(tt.stored, tt.plain)
			if (err != nil) != tt.wantErr {
				t.Errorf("ComparePassword(%q, %q) = %v, wantErr %v", tt.stored, tt.plain, err, tt.wantErr)
			}
		})
	}
}

func TestDestroySession(t *testing.T) {
	store := sessions.NewCookieStore([]byte("test-secret"))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	if err := SetSessionUser(store, req, rec, "alice", false); err != nil {
		t.Fatalf("failed to set session user: %v", err)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Cookie", rec.Header().Get("Set-Cookie"))
	rec2 := httptest.NewRecorder()
	if err := DestroySession(store, req2, rec2); err != nil {
		t.Fatalf("failed to destroy session: %v", err)
	}
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	cookies := rec2.Result().Cookies()
	for _, c := range cookies {
		req3.AddCookie(c)
	}
	rec3 := httptest.NewRecorder()
	username, err := GetSessionUser(store, req3, rec3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if username != "" {
		t.Errorf("expected empty username after session destruction, got %q", username)
	}
}

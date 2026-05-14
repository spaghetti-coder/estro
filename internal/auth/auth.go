package auth

import (
	"errors"
	"net/http"

	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
)

const SessionCookieName = "connect.sid"

const rememberMeMaxAge = 30 * 24 * 3600

func ComparePassword(hashedPassword, plainPassword string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plainPassword))
	if err != nil {
		return errors.New("password does not match")
	}
	return nil
}

func GetSessionUser(store sessions.Store, r *http.Request, w http.ResponseWriter) (string, error) {
	// Error ignored: Get() returns a valid session even on decode errors.
	session, _ := store.Get(r, SessionCookieName)
	username, ok := session.Values["user"].(string)
	if !ok || username == "" {
		return "", nil
	}
	return username, nil
}

func SetSessionUser(store sessions.Store, r *http.Request, w http.ResponseWriter, username string, rememberMe bool) error {
	// Error ignored: Get() always returns a usable session.
	session, _ := store.Get(r, SessionCookieName)
	session.Values["user"] = username
	if rememberMe {
		session.Options = &sessions.Options{
			Path:     "/",
			MaxAge:   rememberMeMaxAge,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		}
	} else {
		session.Options = &sessions.Options{
			Path:     "/",
			MaxAge:   0,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		}
	}
	return session.Save(r, w)
}

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
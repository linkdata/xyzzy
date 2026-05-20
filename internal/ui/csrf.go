package ui

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
)

const csrfFieldName = "csrf"

func (a *App) validCSRF(r *http.Request) (ok bool) {
	cookieValue := a.sessionCookieValue(r)
	if cookieValue != "" {
		if err := r.ParseForm(); err == nil {
			if got, err := base64.RawURLEncoding.DecodeString(r.PostForm.Get(csrfFieldName)); err == nil {
				ok = hmac.Equal(got, a.csrfMAC(cookieValue))
			}
		}
	}
	return
}

func (a *App) csrfToken(cookieValue string) (token string) {
	token = base64.RawURLEncoding.EncodeToString(a.csrfMAC(cookieValue))
	return
}

func (a *App) csrfMAC(cookieValue string) []byte {
	mac := hmac.New(sha256.New, a.csrfSecret[:])
	_, _ = mac.Write([]byte(cookieValue))
	return mac.Sum(nil)
}

func (a *App) sessionCookieValue(r *http.Request) (result string) {
	if r != nil {
		if cookie, err := r.Cookie(a.sessionCookieName()); err == nil {
			result = cookie.Value
		}
	}
	return
}

func (a *App) sessionCookieName() (result string) {
	result = strings.TrimSpace(a.Jaws.CookieName)
	if result == "" {
		result = "jaws"
	}
	return
}

func (a *App) validRequestOrigin(r *http.Request) (ok bool) {
	if r == nil {
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		ok = sameOrigin(origin, r)
		return
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		ok = sameOrigin(referer, r)
		return
	}
	ok = true
	return
}

func sameOrigin(value string, r *http.Request) (ok bool) {
	if u, err := url.Parse(value); err == nil && u.Scheme != "" && u.Host != "" {
		scheme := "http"
		if requestIsSecure(r) {
			scheme = "https"
		}
		ok = strings.EqualFold(u.Scheme, scheme) && strings.EqualFold(u.Host, r.Host)
	}
	return
}

package ui

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLobbyPageRenders(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Pretend You're Xyzzy") || !strings.Contains(body, "nickname-section") {
		t.Fatalf("unexpected lobby body: %s", body)
	}
	if !strings.Contains(body, `rel="icon"`) || app.Jaws.FaviconURL() == "" {
		t.Fatalf("unexpected lobby body: %s", body)
	}
}

func TestLobbyPageSetsNicknameCookieFromSession(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	sess := app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected JaWS session")
	}
	app.setNickname(sess, "Alice")

	req2 := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req2.AddCookie(sess.Cookie())
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	var nicknameCookie *http.Cookie
	for _, cookie := range rec2.Result().Cookies() {
		if cookie.Name == app.nicknameCookieName() {
			nicknameCookie = cookie
			break
		}
	}
	if nicknameCookie == nil {
		t.Fatalf("expected nickname cookie %q to be set", app.nicknameCookieName())
	}
	raw, err := base64.RawURLEncoding.DecodeString(nicknameCookie.Value)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	if got := string(raw); got != "Alice" {
		t.Fatalf("nickname cookie = %q, want %q", got, "Alice")
	}
}

func TestLobbyPageRestoresNicknameFromCookie(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.AddCookie(&http.Cookie{
		Name:  app.nicknameCookieName(),
		Value: base64.RawURLEncoding.EncodeToString([]byte("Alice")),
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	sess := app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected JaWS session")
	}
	if got := app.nickname(sess); got != "Alice" {
		t.Fatalf("nickname(session) = %q, want %q", got, "Alice")
	}
}

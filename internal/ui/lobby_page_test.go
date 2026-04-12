package ui

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linkdata/xyzzy/internal/game"
)

func TestLobbyRenders(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Pretend You're Xyzzy") || !strings.Contains(body, `data-bs-target="#nicknameModal"`) {
		t.Fatalf("unexpected lobby body: %s", body)
	}
	if !strings.Contains(body, `id="nicknameModal"`) || !strings.Contains(body, "Change Nickname") {
		t.Fatalf("expected lobby body to include nickname modal, got %s", body)
	}
	if !strings.Contains(body, "1 online player") {
		t.Fatalf("expected lobby body to show one online player, got %s", body)
	}
	if !strings.Contains(body, `rel="icon"`) || app.Jaws.FaviconURL() == "" {
		t.Fatalf("unexpected lobby body: %s", body)
	}
}

func TestLobbyShowsCurrentOnlinePlayerCount(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	req1 := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	body := rec2.Body.String()
	if !strings.Contains(body, "2 online players") {
		t.Fatalf("expected lobby body to show two online players, got %s", body)
	}
}

func TestLobbySetsNicknameCookieFromPlayer(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	sess := app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected JaWS session")
	}
	player := app.player(sess, req)
	app.setNickname(player, "Alice")

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

func TestLobbyRestoresNicknameFromCookie(t *testing.T) {
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
	player := app.player(sess, req)
	if got := player.Nickname; got != "Alice" {
		t.Fatalf("Nickname = %q, want %q", got, "Alice")
	}
}

func TestLobbyLeavesRoomImmediately(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	sess := app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected JaWS session")
	}
	player := app.player(sess, req)
	app.setNickname(player, "Alice")
	room, err := app.createRoom(player)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	lobbyReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	lobbyReq.AddCookie(sess.Cookie())
	lobbyRec := httptest.NewRecorder()
	handler.ServeHTTP(lobbyRec, lobbyReq)

	if app.Manager.Room(room.Code()) != nil {
		t.Fatalf("expected room %s to be deleted when returning to lobby", room.Code())
	}
	if player.Room != nil {
		t.Fatal("expected player to be in lobby")
	}
}

func TestSetNicknameInRoomKeepsNicknameUnique(t *testing.T) {
	app, _ := testApp(t)
	host := &game.Player{Nickname: "Alice", NicknameInput: "Alice"}
	room, err := app.Manager.CreateRoom(host, app.Catalog.DefaultDeckIDs())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	guest := &game.Player{Nickname: "Bob", NicknameInput: "Bob"}
	if _, err := app.Manager.JoinRoom(room.Code(), guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}

	app.setNickname(guest, "Alice")

	if got := guest.Nickname; got != "Alice-2" {
		t.Fatalf("guest nickname = %q, want %q", got, "Alice-2")
	}
	if got := guest.NicknameInput; got != "Alice-2" {
		t.Fatalf("guest nickname input = %q, want %q", got, "Alice-2")
	}
}

package ui

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/cookiejar"
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

func TestCreateRoomRouteRedirectsToRoom(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	sess := app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected JaWS session")
	}

	createReq := httptest.NewRequest(http.MethodGet, "http://example.test/create-room", nil)
	createReq.AddCookie(sess.Cookie())
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("ServeHTTP() status = %d, want %d", createRec.Code, http.StatusSeeOther)
	}
	location := createRec.Header().Get("Location")
	if !strings.HasPrefix(location, "/room/") {
		t.Fatalf("Location = %q, want /room/<code>", location)
	}

	player := app.player(sess, createReq)
	if player.Room == nil {
		t.Fatal("expected player to be seated in created room")
	}
	if got := app.Manager.Room(player.Room.Code()); got != player.Room {
		t.Fatalf("Manager.Room(%q) = %v, want created room", player.Room.Code(), got)
	}
	if location != "/room/"+player.Room.Code() {
		t.Fatalf("Location = %q, want %q", location, "/room/"+player.Room.Code())
	}
}

func TestCreateRoomRouteFollowRedirectShowsRoomPage(t *testing.T) {
	app, mux := testApp(t)
	server := httptest.NewServer(app.Middleware(mux))
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	client := &http.Client{Jar: jar}

	resp, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET / error = %v", err)
	}
	resp.Body.Close()

	resp, err = client.Get(server.URL + "/create-room")
	if err != nil {
		t.Fatalf("GET /create-room error = %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("final status = %d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(resp.Request.URL.Path, "/room/") {
		t.Fatalf("final URL path = %q, want /room/<code>", resp.Request.URL.Path)
	}
	if !strings.Contains(string(body), "Card Packs") {
		t.Fatalf("expected room page body after redirect, got %s", body)
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

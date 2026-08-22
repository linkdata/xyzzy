package ui

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/game"
)

func TestSessionMapsToSinglePlayer(t *testing.T) {
	app, _ := testApp(t)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	sess := newTestSession(t, app)

	player1 := app.player(sess, req)
	player2 := app.player(sess, req)
	if player1 != player2 {
		t.Fatal("expected one player per JaWS session")
	}
}

func TestCleanupExpiredSessionDeletesEmptyRoom(t *testing.T) {
	h := newLiveHarness(t)

	h.get(t, "/")
	sess := h.session(t)
	player := h.app.player(sess, nil)
	h.app.Manager.SetNickname(player, "Alice")
	room, err := h.app.createRoom(player)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	sess.Close()
	h.app.Manager.CleanupExpiredSessions()

	if h.app.Manager.Room(room.Code()) != nil {
		t.Fatalf("expected room %s to be deleted after expired-session cleanup", room.Code())
	}
}

func TestCleanupExpiredSessionReassignsHost(t *testing.T) {
	h := newLiveHarness(t)

	aliceSess, alice := livePlayer(t, h, "Alice")
	_, bob := liveJoinedPlayer(t, h, "Bob")
	_, casey := liveJoinedPlayer(t, h, "Casey")
	room, err := h.app.createRoom(alice)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}
	if _, err := h.app.joinRoom(bob, room.Code()); err != nil {
		t.Fatalf("joinRoom(Bob) error = %v", err)
	}
	if _, err := h.app.joinRoom(casey, room.Code()); err != nil {
		t.Fatalf("joinRoom(Casey) error = %v", err)
	}

	aliceSess.Close()
	h.app.Manager.CleanupExpiredSessions()

	if room.Host() != bob {
		t.Fatalf("expected Bob to become host, got %#v", room.Host())
	}
}

func TestCleanupExpiredJudgeResetsToLobby(t *testing.T) {
	h := newPlayableLiveHarness(t)

	aliceSess, alice := livePlayer(t, h, "Alice")
	_, bob := liveJoinedPlayer(t, h, "Bob")
	_, casey := liveJoinedPlayer(t, h, "Casey")
	_, drew := liveJoinedPlayer(t, h, "Drew")
	room, err := h.app.createRoom(alice)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}
	for _, player := range []*game.Player{bob, casey, drew} {
		if _, err := h.app.joinRoom(player, room.Code()); err != nil {
			t.Fatalf("joinRoom() error = %v", err)
		}
	}
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	judge := room.JudgePlayer()
	if judge == nil {
		t.Fatal("expected judge")
	}
	var judgeSess *jaws.Session
	switch judge {
	case alice:
		judgeSess = aliceSess
	case bob:
		judgeSess = bob.Session
	case casey:
		judgeSess = casey.Session
	case drew:
		judgeSess = drew.Session
	}
	judgeSess.Close()
	h.app.Manager.CleanupExpiredSessions()

	if room.State() != game.StateLobby {
		t.Fatalf("expected lobby reset after judge cleanup, got %s", room.State())
	}
}

func TestCleanupExpiredPlayerWithTooFewRemainingResetsToLobby(t *testing.T) {
	h := newPlayableLiveHarness(t)

	_, alice := livePlayer(t, h, "Alice")
	bobSess, bob := liveJoinedPlayer(t, h, "Bob")
	_, casey := liveJoinedPlayer(t, h, "Casey")
	room, err := h.app.createRoom(alice)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}
	for _, player := range []*game.Player{bob, casey} {
		if _, err := h.app.joinRoom(player, room.Code()); err != nil {
			t.Fatalf("joinRoom() error = %v", err)
		}
	}
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	bobSess.Close()
	h.app.Manager.CleanupExpiredSessions()

	if room.State() != game.StateLobby {
		t.Fatalf("expected lobby reset after dropping below min players, got %s", room.State())
	}
}

func livePlayer(t *testing.T, h *liveHarness, nickname string) (result1 *jaws.Session, result2 *game.Player) {
	t.Helper()
	h.get(t, "/")
	sess := h.session(t)
	player := h.app.player(sess, nil)
	h.app.Manager.SetNickname(player, nickname)
	result1, result2 = sess, player
	return
}

func liveJoinedPlayer(t *testing.T, h *liveHarness, nickname string) (result1 *jaws.Session, result2 *game.Player) {
	t.Helper()
	client := h.newClient(t)
	h.getWithClient(t, client, "/")
	sess := h.sessionForClient(t, client)
	player := h.app.player(sess, nil)
	h.app.Manager.SetNickname(player, nickname)
	result1, result2 = sess, player
	return
}

func csrfTokenForSession(app *App, sess *jaws.Session) (result string) {
	result = app.csrfToken(sess.CookieValue())
	return
}

func createRoomPostRequest(t *testing.T, token string, cookies []*http.Cookie) (result *http.Request) {
	t.Helper()
	form := url.Values{csrfFieldName: {token}}
	result = httptest.NewRequest(http.MethodPost, "http://example.test/create-room", strings.NewReader(form.Encode()))
	result.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range cookies {
		result.AddCookie(cookie)
	}
	return
}

func TestLobbyPageReceivesLiveRoomUpdates(t *testing.T) {
	h := newLiveHarness(t)

	html := h.get(t, "/")
	conn, cancel := h.connect(t, html)
	defer cancel()

	otherClient := h.newClient(t)
	otherHTML := h.getWithClient(t, otherClient, "/")
	_ = otherHTML
	otherSession := h.sessionForClient(t, otherClient)
	otherPlayer := h.app.player(otherSession, nil)
	h.app.Manager.SetNickname(otherPlayer, "Bob")
	room, err := h.app.createRoom(otherPlayer)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	msg, err := readUntilContains(ctx, conn, room.Code())
	if err != nil {
		t.Fatalf("readUntilContains() error = %v", err)
	}
	if !strings.Contains(msg, "Bob") {
		t.Fatalf("expected lobby update to mention host name, got %s", msg)
	}
}

func TestRoomPageReceivesLiveRoomUpdates(t *testing.T) {
	h := newLiveHarness(t)

	h.get(t, "/")
	sess := h.session(t)
	player := h.app.player(sess, nil)
	h.app.Manager.SetNickname(player, "Alice")
	room, err := h.app.createRoom(player)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	html := h.get(t, "/room/"+room.Code())
	conn, cancel := h.connect(t, html)
	defer cancel()

	if _, err := room.SetDeckEnabled(player, h.app.Catalog.DeckByID("extra"), true); err != nil {
		t.Fatalf("SetDeckEnabled() error = %v", err)
	}
	h.app.Jaws.Dirty(h.app.Manager, room)

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	want := fmt.Sprintf("%d black / %d white selected", 2, 4)
	msg, err := readUntilContains(ctx, conn, want)
	if err != nil {
		t.Fatalf("readUntilContains() error = %v", err)
	}
	if !strings.Contains(msg, "Card Packs") {
		t.Fatalf("expected room update to include deck panel markup, got %s", msg)
	}
}

func TestLobbyPageReceivesLiveRoomRemovalUpdates(t *testing.T) {
	h := newLiveHarness(t)

	html := h.get(t, "/")
	conn, cancel := h.connect(t, html)
	defer cancel()

	otherClient := h.newClient(t)
	h.getWithClient(t, otherClient, "/")
	otherSession := h.sessionForClient(t, otherClient)
	otherPlayer := h.app.player(otherSession, nil)
	h.app.Manager.SetNickname(otherPlayer, "Bob")
	room, err := h.app.createRoom(otherPlayer)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	ctxCreate, doneCreate := context.WithTimeout(context.Background(), 5*time.Second)
	defer doneCreate()
	if _, err := readUntilContains(ctxCreate, conn, room.Code()); err != nil {
		t.Fatalf("readUntilContains(create) error = %v", err)
	}

	h.getWithClient(t, otherClient, "/")

	ctxDelete, doneDelete := context.WithTimeout(context.Background(), 5*time.Second)
	defer doneDelete()
	msg, err := readUntilContains(ctxDelete, conn, "No rooms yet")
	if err != nil {
		t.Fatalf("readUntilContains(delete) error = %v", err)
	}
	if strings.Contains(msg, room.Code()) {
		t.Fatalf("expected room removal update, got %s", msg)
	}
}

func TestLobbyPageReceivesLivePrivateVisibilityUpdates(t *testing.T) {
	h := newLiveHarness(t)

	html := h.get(t, "/")
	conn, cancel := h.connect(t, html)
	defer cancel()

	hostClient := h.newClient(t)
	h.getWithClient(t, hostClient, "/")
	hostSession := h.sessionForClient(t, hostClient)
	host := h.app.player(hostSession, nil)
	h.app.Manager.SetNickname(host, "Bob")
	room, err := h.app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	ctxCreate, doneCreate := context.WithTimeout(context.Background(), 5*time.Second)
	defer doneCreate()
	if _, err := readUntilContains(ctxCreate, conn, room.Code()); err != nil {
		t.Fatalf("readUntilContains(create) error = %v", err)
	}

	privateToggle := room.PrivateToggle(host)
	if err := privateToggle.JawsSet(newPrivateToggleElement(h.app, privateToggle), true); err != nil {
		t.Fatalf("privateToggle.JawsSet(true) error = %v", err)
	}

	ctxHide, doneHide := context.WithTimeout(context.Background(), 5*time.Second)
	defer doneHide()
	hideMsg, err := readUntilContains(ctxHide, conn, "No rooms yet")
	if err != nil {
		t.Fatalf("readUntilContains(hide) error = %v", err)
	}
	if strings.Contains(hideMsg, room.Code()) {
		t.Fatalf("expected private room to disappear from lobby update, got %s", hideMsg)
	}

	if err := privateToggle.JawsSet(newPrivateToggleElement(h.app, privateToggle), false); err != nil {
		t.Fatalf("privateToggle.JawsSet(false) error = %v", err)
	}

	ctxShow, doneShow := context.WithTimeout(context.Background(), 5*time.Second)
	defer doneShow()
	showMsg, err := readUntilContains(ctxShow, conn, room.Code())
	if err != nil {
		t.Fatalf("readUntilContains(show) error = %v", err)
	}
	if !strings.Contains(showMsg, "Bob") {
		t.Fatalf("expected room reappearance update to mention host name, got %s", showMsg)
	}
}

func TestCreateRoomWhileLobbyIsLiveStillOpensRoomPage(t *testing.T) {
	h := newLiveHarness(t)

	html := h.get(t, "/")
	conn, cancel := h.connect(t, html)
	defer cancel()
	_ = conn
	sess := h.session(t)

	resp, err := h.client.PostForm(h.server.URL+"/create-room", url.Values{csrfFieldName: {csrfTokenForSession(h.app, sess)}})
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

func newPrivateToggleElement(app *App, toggle bind.Binder[bool]) (result *jaws.Element) {
	result = app.Jaws.NewRequest(httptest.NewRecorder(), nil).NewElement(jui.NewCheckbox(toggle))
	return
}

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
	sess := app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected lobby GET to establish a JaWS session")
	}
	if player, _ := sess.Get(sessionKeyPlayer).(*game.Player); player == nil {
		t.Fatal("expected lobby GET to store its Player before rendering")
	}
	if !strings.Contains(body, ">1 online</small>") {
		t.Fatalf("expected lobby body to show one online session, got %s", body)
	}
	if !strings.Contains(body, `rel="icon"`) || app.Jaws.FaviconURL() == "" {
		t.Fatalf("unexpected lobby body: %s", body)
	}
	if !strings.Contains(body, "Create Room") {
		t.Fatalf("expected lobby body to include create-room button, got %s", body)
	}
}

func TestLobbyShowsCurrentOnlineSessionCount(t *testing.T) {
	h := newLiveHarness(t)

	firstHTML := h.get(t, "/")
	if !strings.Contains(firstHTML, ">1 online</small>") {
		t.Fatalf("expected first page to show one online session, got %s", firstHTML)
	}
	session := h.session(t)
	if count := h.app.Jaws.SessionCount(); count != 1 {
		t.Fatalf("SessionCount() = %d, want 1", count)
	}
	_, firstCancel := h.connect(t, firstHTML)
	defer firstCancel()

	secondHTML := h.get(t, "/")
	if h.session(t) != session {
		t.Fatal("second tab did not reuse the first tab's session")
	}
	if !strings.Contains(secondHTML, ">1 online</small>") {
		t.Fatalf("expected second tab to keep one online session, got %s", secondHTML)
	}
	_, secondCancel := h.connect(t, secondHTML)
	defer secondCancel()

	if body := h.get(t, "/"); !strings.Contains(body, ">1 online</small>") {
		t.Fatalf("expected two live tabs to keep one online session, got %s", body)
	}

	otherClient := h.newClient(t)
	if body := h.getWithClient(t, otherClient, "/"); !strings.Contains(body, ">2 online</small>") {
		t.Fatalf("expected a second client to show two online sessions, got %s", body)
	}
	if h.sessionForClient(t, otherClient) == session {
		t.Fatal("second client unexpectedly reused the first client's session")
	}
	if count := h.app.Jaws.SessionCount(); count != 2 {
		t.Fatalf("SessionCount() = %d, want 2", count)
	}
}

func TestPageHandlersRequireSessionMiddleware(t *testing.T) {
	app, _ := testApp(t)
	tests := []struct {
		name  string
		path  string
		serve http.HandlerFunc
	}{
		{name: "lobby", path: "http://example.test/", serve: app.serveLobby},
		{name: "room", path: "http://example.test/room/MISSING", serve: app.serveRoom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.name == "room" {
				req.SetPathValue("code", "MISSING")
			}
			rec := httptest.NewRecorder()

			tt.serve.ServeHTTP(rec, req)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("ServeHTTP() status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
			}
		})
	}
}

func TestAnonymousRoomPageJoinsBeforeRendering(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	hostSession := newTestSession(t, app)
	host := app.player(hostSession, nil)
	app.Manager.SetNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d, want %d", rec.Code, http.StatusOK)
	}
	sess := app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected room GET to establish a JaWS session")
	}
	player, _ := sess.Get(sessionKeyPlayer).(*game.Player)
	if player == nil || player.Room() != room || !room.HasPlayer(player) {
		t.Fatalf("expected room GET to seat its Player, got %#v", player)
	}
	if room.PlayerCount() != 2 {
		t.Fatalf("PlayerCount() = %d, want 2", room.PlayerCount())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Card Packs") || !strings.Contains(body, room.Code()) {
		t.Fatalf("expected initial room document to contain seated regions, got %s", body)
	}
}

func TestStaticMetadataAndUnknownRoutes(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	tests := []struct {
		path        string
		wantStatus  int
		contentType string
	}{
		{path: "/robots.txt", wantStatus: http.StatusOK, contentType: "text/plain"},
		{path: "/.well-known/security.txt", wantStatus: http.StatusOK, contentType: "text/plain"},
		{path: "/.env", wantStatus: http.StatusNotFound},
		{path: "/admin", wantStatus: http.StatusNotFound},
		{path: "/openapi.json", wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "http://example.test"+tt.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tt.wantStatus {
			t.Fatalf("GET %s status = %d, want %d", tt.path, rec.Code, tt.wantStatus)
		}
		if tt.contentType != "" && !strings.HasPrefix(rec.Header().Get("Content-Type"), tt.contentType) {
			t.Fatalf("GET %s Content-Type = %q, want prefix %q", tt.path, rec.Header().Get("Content-Type"), tt.contentType)
		}
	}
}

func TestLobbySetsNicknameCookieFromPlayer(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	sess := newTestSession(t, app)
	player := app.player(sess, req)
	app.Manager.SetNickname(player, "Alice")

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
		t.Fatal("expected lobby GET to establish a JaWS session")
	}
	player, _ := sess.Get(sessionKeyPlayer).(*game.Player)
	if player == nil || player.NicknameValue() != "Alice" {
		t.Fatalf("expected restored nickname on session Player, got %#v", player)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Alice") {
		t.Fatalf("expected restored nickname in lobby body, got %s", body)
	}
}

func TestLobbyLeavesRoomImmediately(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	sess := newTestSession(t, app)
	player := app.player(sess, req)
	app.Manager.SetNickname(player, "Alice")
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
	if player.Room() != nil {
		t.Fatal("expected player to be in lobby")
	}
}

func TestCreateRoomRouteRedirectsToRoom(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	sess := newTestSession(t, app)
	token := csrfTokenForSession(app, sess)

	createReq := createRoomPostRequest(t, token, []*http.Cookie{sess.Cookie()})
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
	playerRoom := player.Room()
	if playerRoom == nil {
		t.Fatal("expected player to be seated in created room")
	}
	if got := app.Manager.Room(playerRoom.Code()); got != playerRoom {
		t.Fatalf("Manager.Room(%q) = %v, want created room", playerRoom.Code(), got)
	}
	if location != "/room/"+playerRoom.Code() {
		t.Fatalf("Location = %q, want %q", location, "/room/"+playerRoom.Code())
	}
}

func TestCreateRoomRejectsUnsafeRequests(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req := httptest.NewRequest(method, "http://example.test/create-room", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s /create-room status = %d, want %d", method, rec.Code, http.StatusMethodNotAllowed)
		}
		if got := rec.Header().Get("Allow"); got != http.MethodPost {
			t.Fatalf("%s /create-room Allow = %q, want POST", method, got)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.test/create-room", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /create-room without token status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if rooms := app.Manager.PublicRooms(); len(rooms) != 0 {
		t.Fatalf("unexpected room creation after rejected requests: %v", rooms)
	}
}

func TestCreateRoomRejectsCrossOriginPost(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	sess := newTestSession(t, app)

	createReq := createRoomPostRequest(t, csrfTokenForSession(app, sess), []*http.Cookie{sess.Cookie()})
	createReq.Header.Set("Origin", "https://evil.example")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin POST status = %d, want %d", createRec.Code, http.StatusForbidden)
	}
	if rooms := app.Manager.PublicRooms(); len(rooms) != 0 {
		t.Fatalf("unexpected room creation after cross-origin POST: %v", rooms)
	}
}

func TestCreateRoomRateLimit(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	sessReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	sessReq.RemoteAddr = "203.0.113.9:12345"
	sessRec := httptest.NewRecorder()
	sess := app.Jaws.NewSession(sessRec, sessReq)
	token := csrfTokenForSession(app, sess)
	cookies := []*http.Cookie{sess.Cookie()}

	var limited *httptest.ResponseRecorder
	for range 11 {
		createReq := createRoomPostRequest(t, token, cookies)
		createReq.RemoteAddr = "203.0.113.9:12345"
		createRec := httptest.NewRecorder()
		handler.ServeHTTP(createRec, createReq)
		if createRec.Code == http.StatusTooManyRequests {
			limited = createRec
			break
		}
	}
	if limited == nil {
		t.Fatal("expected create-room rate limit to return 429")
	}
	if got := limited.Header().Get("Retry-After"); got == "" {
		t.Fatal("expected Retry-After header on rate-limited response")
	}
}

func TestCreateRoomRouteFollowRedirectShowsRoomPage(t *testing.T) {
	h := newLiveHarness(t)

	html := h.get(t, "/")
	conn, cancel := h.connect(t, html)
	defer cancel()
	_ = conn
	sess := h.session(t)

	resp, err := h.client.PostForm(h.server.URL+"/create-room", url.Values{csrfFieldName: {csrfTokenForSession(h.app, sess)}})
	if err != nil {
		t.Fatalf("POST /create-room error = %v", err)
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
	room, err := app.Manager.CreateRoom(host, app.Catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	guest := &game.Player{Nickname: "Bob", NicknameInput: "Bob"}
	if _, err := app.Manager.JoinRoom(room.Code(), guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}

	app.Manager.SetNickname(guest, "Alice")

	if got := guest.Nickname; got != "Alice-2" {
		t.Fatalf("guest nickname = %q, want %q", got, "Alice-2")
	}
	if got := guest.NicknameInput; got != "Alice-2" {
		t.Fatalf("guest nickname input = %q, want %q", got, "Alice-2")
	}
}

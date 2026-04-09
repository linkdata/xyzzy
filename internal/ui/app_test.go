package ui

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

func testCatalog(t *testing.T) *deck.Catalog {
	t.Helper()
	fsys := fstest.MapFS{
		"assets/cards/black/b1.json":    {Data: []byte(`{"id":"b1","text":"Question?"}`)},
		"assets/cards/black/b2.json":    {Data: []byte(`{"id":"b2","text":"Another question?"}`)},
		"assets/cards/white/w1.json":    {Data: []byte(`{"id":"w1","text":"Answer 1"}`)},
		"assets/cards/white/w2.json":    {Data: []byte(`{"id":"w2","text":"Answer 2"}`)},
		"assets/cards/white/w3.json":    {Data: []byte(`{"id":"w3","text":"Answer 3"}`)},
		"assets/cards/white/w4.json":    {Data: []byte(`{"id":"w4","text":"Answer 4"}`)},
		"assets/decks/base/deck.json":   {Data: []byte(`{"id":"base","name":"Base","enabled_by_default":true}`)},
		"assets/decks/base/black.json":  {Data: []byte(`["b1"]`)},
		"assets/decks/base/white.json":  {Data: []byte(`["w1","w2","w3"]`)},
		"assets/decks/extra/deck.json":  {Data: []byte(`{"id":"extra","name":"Extra"}`)},
		"assets/decks/extra/black.json": {Data: []byte(`["b2"]`)},
		"assets/decks/extra/white.json": {Data: []byte(`["w4"]`)},
	}
	catalog, err := deck.LoadFS(fsys)
	if err != nil {
		t.Fatalf("LoadFS() error = %v", err)
	}
	return catalog
}

func testApp(t *testing.T) (*App, *http.ServeMux) {
	t.Helper()
	jw, err := jaws.New()
	if err != nil {
		t.Fatalf("jaws.New() error = %v", err)
	}
	t.Cleanup(jw.Close)
	app := New(jw, testCatalog(t), game.NewManager(testCatalog(t)))
	mux := http.NewServeMux()
	if err := app.SetupRoutes(mux); err != nil {
		t.Fatalf("SetupRoutes() error = %v", err)
	}
	return app, mux
}

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

func TestRoomPageRendersExistingRoom(t *testing.T) {
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
	room, err := app.createRoom(sess)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	roomReq.SetPathValue("code", room.Code())
	roomReq.AddCookie(sess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	body := roomRec.Body.String()
	if !strings.Contains(body, room.Code()) || !strings.Contains(body, "Card Packs") {
		t.Fatalf("unexpected room body: %s", body)
	}
}

func TestRoomPageAutoJoinSkipsRedundantSuccessAlert(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	hostReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	hostRec := httptest.NewRecorder()
	handler.ServeHTTP(hostRec, hostReq)
	hostSess := app.Jaws.GetSession(hostReq)
	if hostSess == nil {
		t.Fatal("expected JaWS session")
	}
	app.setNickname(hostSess, "Alice")
	room, err := app.createRoom(hostSess)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	joinReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	joinRec := httptest.NewRecorder()
	handler.ServeHTTP(joinRec, joinReq)
	joinSess := app.Jaws.GetSession(joinReq)
	if joinSess == nil {
		t.Fatal("expected JaWS session")
	}
	app.setNickname(joinSess, "Bob")

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	roomReq.SetPathValue("code", room.Code())
	roomReq.AddCookie(joinSess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	body := roomRec.Body.String()
	if strings.Contains(body, "Joined room") {
		t.Fatalf("unexpected redundant join alert: %s", body)
	}
}

func TestMissingRoomSkipsRedundantAlert(t *testing.T) {
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

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/MISSING", nil)
	roomReq.SetPathValue("code", "MISSING")
	roomReq.AddCookie(sess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	body := roomRec.Body.String()
	if strings.Contains(body, `<p class="notice room-notice">room not found</p>`) {
		t.Fatalf("unexpected duplicate missing-room alert: %s", body)
	}
	if !strings.Contains(body, "Room not found") {
		t.Fatalf("expected missing-room panel text: %s", body)
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

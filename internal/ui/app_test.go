package ui

import (
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
		"assets/cards/black/b1.json":   {Data: []byte(`{"id":"b1","text":"Question?"}`)},
		"assets/cards/white/w1.json":   {Data: []byte(`{"id":"w1","text":"Answer 1"}`)},
		"assets/cards/white/w2.json":   {Data: []byte(`{"id":"w2","text":"Answer 2"}`)},
		"assets/cards/white/w3.json":   {Data: []byte(`{"id":"w3","text":"Answer 3"}`)},
		"assets/decks/base/deck.json":  {Data: []byte(`{"id":"base","name":"Base","enabled_by_default":true}`)},
		"assets/decks/base/black.json": {Data: []byte(`["b1"]`)},
		"assets/decks/base/white.json": {Data: []byte(`["w1","w2","w3"]`)},
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
	app := New(jw, testCatalog(t), game.NewManager(testCatalog(t), nil))
	mux := http.NewServeMux()
	if err := app.SetupRoutes(mux); err != nil {
		t.Fatalf("SetupRoutes() error = %v", err)
	}
	return app, mux
}

func TestLobbyPageRenders(t *testing.T) {
	_, mux := testApp(t)
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Pretend You're Xyzzy") || !strings.Contains(body, "Choose a nickname") {
		t.Fatalf("unexpected lobby body: %s", body)
	}
}

func TestRoomPageRendersExistingRoom(t *testing.T) {
	app, mux := testApp(t)
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	sess := app.ensureSession(rec, req)
	app.setNickname(sess, "Alice")
	room, err := app.createRoom(sess)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	roomReq.SetPathValue("code", room.Code())
	roomReq.AddCookie(sess.Cookie())
	roomRec := httptest.NewRecorder()
	mux.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	body := roomRec.Body.String()
	if !strings.Contains(body, room.Code()) || !strings.Contains(body, "Deck Selection") {
		t.Fatalf("unexpected room body: %s", body)
	}
}

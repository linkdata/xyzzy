package ui

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/linkdata/xyzzy/internal/game"
)

func TestRoomRendersExistingRoom(t *testing.T) {
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
	if !strings.Contains(body, "Private game") || !strings.Contains(body, "private-game-group") {
		t.Fatalf("expected lobby controls to include private-game input group: %s", body)
	}
	privateToggle := regexp.MustCompile(`<input[^>]*class="form-check-input private-toggle-checkbox mt-0 me-1"[^>]*>`)
	match := privateToggle.FindString(body)
	if match == "" || strings.Contains(match, `checked`) {
		t.Fatalf("expected private checkbox to render unchecked by default, got %q", match)
	}
	if !(strings.Contains(body, "Target score") && strings.Contains(body, "Start Game")) {
		t.Fatalf("expected unified lobby controls to include target score and start button: %s", body)
	}
	if !strings.Contains(body, `row row-cols-1 row-cols-md-3 g-2`) {
		t.Fatalf("expected deck selection grid to render three columns at the normal breakpoint: %s", body)
	}
}

func TestRoomAutoJoinsLobbyRoom(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	hostReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	hostRec := httptest.NewRecorder()
	handler.ServeHTTP(hostRec, hostReq)
	hostSess := app.Jaws.GetSession(hostReq)
	if hostSess == nil {
		t.Fatal("expected JaWS session")
	}
	host := app.player(hostSess, hostReq)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
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
	guest := app.player(joinSess, joinReq)
	app.setNickname(guest, "Bob")

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	roomReq.SetPathValue("code", room.Code())
	roomReq.AddCookie(joinSess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	if guest.Room != room {
		t.Fatal("expected guest to auto-join lobby room")
	}
	body := roomRec.Body.String()
	if !strings.Contains(body, "Card Packs") {
		t.Fatalf("expected joined room body, got %s", body)
	}
}

func TestPrivateRoomStillAutoJoinsByDirectURL(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	hostReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	hostRec := httptest.NewRecorder()
	handler.ServeHTTP(hostRec, hostReq)
	hostSess := app.Jaws.GetSession(hostReq)
	if hostSess == nil {
		t.Fatal("expected JaWS session")
	}
	host := app.player(hostSess, hostReq)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}
	if err := room.SetPrivate(host, true); err != nil {
		t.Fatalf("SetPrivate() error = %v", err)
	}

	joinReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	joinRec := httptest.NewRecorder()
	handler.ServeHTTP(joinRec, joinReq)
	joinSess := app.Jaws.GetSession(joinReq)
	if joinSess == nil {
		t.Fatal("expected JaWS session")
	}
	guest := app.player(joinSess, joinReq)
	app.setNickname(guest, "Bob")

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	roomReq.SetPathValue("code", room.Code())
	roomReq.AddCookie(joinSess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	if guest.Room != room {
		t.Fatal("expected guest to auto-join private room by direct URL")
	}
	if body := roomRec.Body.String(); !strings.Contains(body, "Card Packs") {
		t.Fatalf("expected private room body, got %s", body)
	}
}

func TestRoomAutoJoinsGameInProgress(t *testing.T) {
	app, mux := testPlayableApp(t)
	handler := app.Middleware(mux)

	hostReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	hostRec := httptest.NewRecorder()
	handler.ServeHTTP(hostRec, hostReq)
	hostSess := app.Jaws.GetSession(hostReq)
	if hostSess == nil {
		t.Fatal("expected JaWS session")
	}
	host := app.player(hostSess, hostReq)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guest1Sess := newTestSession(t, app, handler)
	guest1 := app.player(guest1Sess, nil)
	app.setNickname(guest1, "Bob")
	if _, err := app.joinRoom(guest1, room.Code()); err != nil {
		t.Fatalf("JoinRoom(guest1) error = %v", err)
	}
	if err := room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	joinReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	joinRec := httptest.NewRecorder()
	handler.ServeHTTP(joinRec, joinReq)
	joinSess := app.Jaws.GetSession(joinReq)
	if joinSess == nil {
		t.Fatal("expected JaWS session")
	}
	guest := app.player(joinSess, joinReq)
	app.setNickname(guest, "Drew")

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	roomReq.SetPathValue("code", room.Code())
	roomReq.AddCookie(joinSess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	if guest.Room != room {
		t.Fatal("expected guest to auto-join game in progress")
	}
	body := roomRec.Body.String()
	if !strings.Contains(body, "Your Hand") {
		t.Fatalf("expected in-progress room body for joined player, got %s", body)
	}
}

func TestMissingRoomRendersMissingPanel(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	sess := app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected JaWS session")
	}

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/MISSING", nil)
	roomReq.SetPathValue("code", "MISSING")
	roomReq.AddCookie(sess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	body := roomRec.Body.String()
	if !strings.Contains(body, "Room not found") {
		t.Fatalf("expected missing-room panel text: %s", body)
	}
}

func TestRoomRedirectsToCurrentRoom(t *testing.T) {
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

	other := &game.Player{Nickname: "Bob", NicknameInput: "Bob"}
	otherRoom, err := app.Manager.CreateRoom(other, app.Catalog.DefaultDeckIDs())
	if err != nil {
		t.Fatalf("CreateRoom(other) error = %v", err)
	}

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+otherRoom.Code(), nil)
	roomReq.SetPathValue("code", otherRoom.Code())
	roomReq.AddCookie(sess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)

	if roomRec.Code != http.StatusSeeOther {
		t.Fatalf("ServeHTTP() status = %d, want %d", roomRec.Code, http.StatusSeeOther)
	}
	if got := roomRec.Header().Get("Location"); got != "/room/"+room.Code() {
		t.Fatalf("Location = %q, want %q", got, "/room/"+room.Code())
	}
}

func TestApplyCardSelectionReplacesSinglePickSelection(t *testing.T) {
	player := &game.Player{SelectedCardIDs: []string{"w1"}}

	changed, alert := applyCardSelection(player, "w2", 1)

	if len(player.SelectedCardIDs) != 1 || player.SelectedCardIDs[0] != "w2" {
		t.Fatalf("SelectedCardIDs = %v, want [w2]", player.SelectedCardIDs)
	}
	if !changed || alert != "" {
		t.Fatalf("applyCardSelection() = (%v, %q), want (true, \"\")", changed, alert)
	}
}

func TestApplyCardSelectionKeepsMultiPickLimit(t *testing.T) {
	player := &game.Player{SelectedCardIDs: []string{"w1", "w2"}}

	changed, alert := applyCardSelection(player, "w3", 2)

	if len(player.SelectedCardIDs) != 2 || player.SelectedCardIDs[0] != "w1" || player.SelectedCardIDs[1] != "w2" {
		t.Fatalf("SelectedCardIDs = %v, want unchanged", player.SelectedCardIDs)
	}
	if changed || alert == "" {
		t.Fatalf("applyCardSelection() = (%v, %q), want validation message without mutation", changed, alert)
	}
}

package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linkdata/xyzzy/internal/game"
)

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

func TestApplyCardSelectionReplacesSinglePickSelection(t *testing.T) {
	page := &RoomPage{SelectedCardIDs: []string{"w1"}}

	page.applyCardSelection("w2", 1)

	if len(page.SelectedCardIDs) != 1 || page.SelectedCardIDs[0] != "w2" {
		t.Fatalf("SelectedCardIDs = %v, want [w2]", page.SelectedCardIDs)
	}
	if page.Alert != "" {
		t.Fatalf("Alert = %q, want empty", page.Alert)
	}
}

func TestApplyCardSelectionKeepsMultiPickLimit(t *testing.T) {
	page := &RoomPage{SelectedCardIDs: []string{"w1", "w2"}}

	page.applyCardSelection("w3", 2)

	if len(page.SelectedCardIDs) != 2 || page.SelectedCardIDs[0] != "w1" || page.SelectedCardIDs[1] != "w2" {
		t.Fatalf("SelectedCardIDs = %v, want unchanged", page.SelectedCardIDs)
	}
	if page.Alert == "" {
		t.Fatal("Alert = empty, want validation message")
	}
}

func TestWaitingTitleForJudgeDuringPlay(t *testing.T) {
	snap := game.RoomView{
		State: game.StatePlaying,
		Players: []game.PlayerView{
			{ID: "p1", IsJudge: true},
			{ID: "p2"},
		},
	}

	if got := waitingTitle(snap, "p1"); got != "Waiting for answers" {
		t.Fatalf("waitingTitle() = %q", got)
	}
	if got := waitingDetail(snap, "p1"); got != "You'll choose the winner once every answer is in." {
		t.Fatalf("waitingDetail() = %q", got)
	}
}

func TestWaitingTitleForSubmittedPlayerDuringPlay(t *testing.T) {
	snap := game.RoomView{
		State: game.StatePlaying,
		Players: []game.PlayerView{
			{ID: "p1", Submitted: true},
			{ID: "p2"},
		},
	}

	if got := waitingTitle(snap, "p1"); got != "Waiting for the rest of the table" {
		t.Fatalf("waitingTitle() = %q", got)
	}
	if got := waitingDetail(snap, "p1"); got != "Your cards are in." {
		t.Fatalf("waitingDetail() = %q", got)
	}
}

func TestWaitingTitleDuringJudging(t *testing.T) {
	snap := game.RoomView{
		State:     game.StateJudging,
		JudgeName: "Casey",
	}

	if got := waitingTitle(snap, "p1"); got != "Casey is picking the winner" {
		t.Fatalf("waitingTitle() = %q", got)
	}
	if got := waitingDetail(snap, "p1"); got != "" {
		t.Fatalf("waitingDetail() = %q", got)
	}
}

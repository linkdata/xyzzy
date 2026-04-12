package ui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	jui "github.com/linkdata/jaws/lib/ui"
)

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
	h.app.setNickname(otherPlayer, "Bob")
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
	h.app.setNickname(player, "Alice")
	room, err := h.app.createRoom(player)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	html := h.get(t, "/room/"+room.Code())
	conn, cancel := h.connect(t, html)
	defer cancel()

	if err := room.SetDeckEnabled(player, h.app.Catalog.DeckByID("extra"), true); err != nil {
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
	h.app.setNickname(otherPlayer, "Bob")
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
	h.app.setNickname(host, "Bob")
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

	resp, err := h.client.Get(h.server.URL + "/create-room")
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

func newPrivateToggleElement(app *App, toggle bind.Binder[bool]) *jaws.Element {
	return app.Jaws.NewRequest(nil).NewElement(jui.NewCheckbox(toggle))
}

package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
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
	h.app.DirtyRoom(room)

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

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
	h.getWithClient(t, otherClient, "/")
	otherSession := h.sessionForClient(t, otherClient)
	h.app.setNickname(otherSession, "Bob")
	room, err := h.app.createRoom(otherSession)
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
	h.app.setNickname(sess, "Alice")
	room, err := h.app.createRoom(sess)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	html := h.get(t, "/room/"+room.Code())
	conn, cancel := h.connect(t, html)
	defer cancel()

	if err := room.ToggleDeck(h.app.playerID(sess), "extra", true); err != nil {
		t.Fatalf("ToggleDeck() error = %v", err)
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

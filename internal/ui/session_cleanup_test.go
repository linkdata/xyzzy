package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/game"
)

func TestSessionMapsToSinglePlayer(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	sess := app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected JaWS session")
	}

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
	h.app.setNickname(player, "Alice")
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

func livePlayer(t *testing.T, h *liveHarness, nickname string) (*jaws.Session, *game.Player) {
	t.Helper()
	h.get(t, "/")
	sess := h.session(t)
	player := h.app.player(sess, nil)
	h.app.setNickname(player, nickname)
	return sess, player
}

func liveJoinedPlayer(t *testing.T, h *liveHarness, nickname string) (*jaws.Session, *game.Player) {
	t.Helper()
	client := h.newClient(t)
	h.getWithClient(t, client, "/")
	sess := h.sessionForClient(t, client)
	player := h.app.player(sess, nil)
	h.app.setNickname(player, nickname)
	return sess, player
}

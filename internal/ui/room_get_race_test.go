package ui

import (
	"context"
	"sync"
	"testing"

	"github.com/linkdata/jaws/lib/what"
	"github.com/linkdata/jaws/lib/wire"
	"github.com/linkdata/xyzzy/internal/game"
)

func TestConcurrentSessionPlayerLookupReturnsOnePlayer(t *testing.T) {
	app, _ := testApp(t)
	const (
		trials  = 100
		workers = 32
	)

	for trial := 0; trial < trials; trial++ {
		sess := newTestSession(t, app)
		start := make(chan struct{})
		players := make([]*game.Player, workers)
		var wg sync.WaitGroup
		wg.Add(workers)
		for i := range players {
			go func() {
				defer wg.Done()
				<-start
				players[i] = app.player(sess, nil)
			}()
		}
		close(start)
		wg.Wait()

		stored, _ := sess.Get(sessionKeyPlayer).(*game.Player)
		if stored == nil {
			t.Fatalf("trial %d: Session has no Player", trial)
		}
		for i, player := range players {
			if player != stored {
				t.Fatalf("trial %d worker %d: player = %p, stored = %p", trial, i, player, stored)
			}
		}
		sess.Close()
	}
}

func TestSecondRoomConnectionRedirectsToCurrentRoom(t *testing.T) {
	h := newLiveHarness(t)
	_, firstHost := livePlayer(t, h, "FirstHost")
	firstRoom, err := h.app.createRoom(firstHost)
	if err != nil {
		t.Fatalf("createRoom(first host) error = %v", err)
	}
	_, secondHost := liveJoinedPlayer(t, h, "SecondHost")
	secondRoom, err := h.app.createRoom(secondHost)
	if err != nil {
		t.Fatalf("createRoom(second host) error = %v", err)
	}

	client := h.newClient(t)
	firstHTML := h.getWithClient(t, client, h.app.RoomURL(firstRoom.Code()))
	secondHTML := h.getWithClient(t, client, h.app.RoomURL(secondRoom.Code()))
	sess := h.sessionForClient(t, client)
	player := h.app.player(sess, nil)
	if player.Room() != nil {
		t.Fatal("room GETs seated the viewer")
	}

	firstConn, firstCancel := h.connectWithClient(t, client, firstHTML)
	defer firstCancel()
	firstReader := newImmediateModeWireReader(t, firstConn)
	firstRequest := immediateModeRequestForHTML(t, sess, firstHTML)
	syncImmediateModeRequest(t, firstConn, firstRequest, firstReader, "first-room-connected")
	if player.Room() != firstRoom {
		t.Fatalf("Room() = %v, want first room %v", player.Room(), firstRoom)
	}

	secondConn, secondCancel := h.connectWithClient(t, client, secondHTML)
	defer secondCancel()
	secondReader := newImmediateModeWireReader(t, secondConn)
	ctx, cancel := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	if err = secondReader.readUntil(ctx, func(msg wire.WsMsg) bool {
		return msg.What == what.Redirect && msg.Data == h.app.RoomURL(firstRoom.Code())
	}); err != nil {
		cancel()
		t.Fatalf("waiting for second room redirect: %v", err)
	}
	cancel()

	if player.Room() != firstRoom || !firstRoom.HasPlayer(player) || secondRoom.HasPlayer(player) {
		t.Fatalf("player membership after second connect: current=%v first=%v second=%v", player.Room(), firstRoom.HasPlayer(player), secondRoom.HasPlayer(player))
	}
}

func TestRoomConnectionReloadsAfterCapturedRoomRemoval(t *testing.T) {
	h := newLiveHarness(t)
	_, host := livePlayer(t, h, "Host")
	room, err := h.app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	client := h.newClient(t)
	pageHTML := h.getWithClient(t, client, h.app.RoomURL(room.Code()))
	sess := h.sessionForClient(t, client)
	player := h.app.player(sess, nil)
	if player.Room() != nil {
		t.Fatal("room GET seated the viewer")
	}
	if left := h.app.leaveRoom(host); left != room {
		t.Fatalf("leaveRoom(host) = %v, want %v", left, room)
	}
	if current := h.app.Manager.Room(room.Code()); current != nil {
		t.Fatalf("Manager.Room(%q) = %v, want nil", room.Code(), current)
	}

	conn, cancel := h.connectWithClient(t, client, pageHTML)
	defer cancel()
	reader := newImmediateModeWireReader(t, conn)
	ctx, done := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer done()
	want := h.app.RoomURL(room.Code())
	if err = reader.readUntil(ctx, func(msg wire.WsMsg) bool {
		return msg.What == what.Redirect && msg.Data == want
	}); err != nil {
		t.Fatalf("waiting for removed-room reload to %q: %v", want, err)
	}
	if player.Room() != nil {
		t.Fatalf("Room() after stale-page connect = %v, want nil", player.Room())
	}
}

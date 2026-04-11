package game

import "testing"

func TestManagerRoomLifecycle(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	room, err := mgr.CreateRoom("p1", "Alice", catalog.DefaultDeckIDs())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := mgr.JoinRoom(room.Code(), "p2", "Bob"); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if _, err := mgr.JoinRoom(room.Code(), "p3", "Casey"); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if err := room.Start("p1"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	view := room.Snapshot("p2")
	if !view.InRoom || view.State != StatePlaying {
		t.Fatalf("Snapshot() = %#v", view)
	}
	if len(view.Hand) < HandSize {
		t.Fatalf("expected hand size >= %d, got %d", HandSize, len(view.Hand))
	}
	if view.CurrentBlack == nil {
		t.Fatal("expected current black card")
	}
}

func TestDebugMinPlayersAllowsTwoPlayerStart(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManagerWithOptions(catalog, Options{MinPlayers: 2})
	room, err := mgr.CreateRoom("p1", "Alice", []string{"base", "expansion"})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := mgr.JoinRoom(room.Code(), "p2", "Bob"); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if err := room.Start("p1"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if snap := room.Snapshot("p1"); snap.State != StatePlaying || len(snap.Players) != 2 || snap.CurrentBlack == nil {
		t.Fatalf("unexpected two-player snapshot: %#v", snap)
	}
}

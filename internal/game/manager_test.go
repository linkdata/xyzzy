package game

import "testing"

func TestManagerRoomLifecycle(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")

	room, err := mgr.CreateRoom(alice, catalog.DefaultDeckIDs())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := mgr.JoinRoom(room.Code(), bob); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if _, err := mgr.JoinRoom(room.Code(), casey); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if room.State() != StatePlaying {
		t.Fatalf("State() = %s, want %s", room.State(), StatePlaying)
	}
	if room.PlayerCount() != 3 {
		t.Fatalf("PlayerCount() = %d, want 3", room.PlayerCount())
	}
	if len(room.HandFor(bob)) < HandSize {
		t.Fatalf("expected hand size >= %d, got %d", HandSize, len(room.HandFor(bob)))
	}
	if room.CurrentBlack() == nil {
		t.Fatal("expected current black card")
	}
}

func TestDebugMinPlayersAllowsTwoPlayerStart(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManagerWithOptions(catalog, Options{MinPlayers: 2})
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")

	room, err := mgr.CreateRoom(alice, []string{"base", "expansion"})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := mgr.JoinRoom(room.Code(), bob); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if room.State() != StatePlaying || room.PlayerCount() != 2 || room.CurrentBlack() == nil {
		t.Fatalf("unexpected two-player room state")
	}
}

func TestDebugStartUsesHighestPickBlackCardFirst(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManagerWithOptions(catalog, Options{MinPlayers: 2, Debug: true})
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")

	room, err := mgr.CreateRoom(alice, []string{"base", "expansion"})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := mgr.JoinRoom(room.Code(), bob); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := room.CurrentBlack(); got == nil || got.ID != "b2" {
		t.Fatalf("CurrentBlack() = %#v, want b2 as highest-pick opening card", got)
	}
}

func TestNicknameSanitizationAndConflictSuffixing(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	host := testPlayer("Alice!!!")
	other := testPlayer("A l i c e")
	third := testPlayer("!!!")

	room, err := mgr.CreateRoom(host, catalog.DefaultDeckIDs())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := mgr.JoinRoom(room.Code(), other); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if _, err := mgr.JoinRoom(room.Code(), third); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}

	if host.Nickname != "Alice" {
		t.Fatalf("host nickname = %q, want %q", host.Nickname, "Alice")
	}
	if other.Nickname != "Alice-2" {
		t.Fatalf("other nickname = %q, want %q", other.Nickname, "Alice-2")
	}
	if third.Nickname != "Player" {
		t.Fatalf("third nickname = %q, want %q", third.Nickname, "Player")
	}
}

package game

import (
	"testing"

	"github.com/linkdata/xyzzy/internal/deck"
)

func TestManagerRoomLifecycle(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")

	room, err := mgr.CreateRoom(alice, catalog.DefaultDecks())
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

	room, err := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
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

	room, err := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
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

func TestRoomPrivacyLifecycleAndPublicRooms(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManagerWithOptions(catalog, Options{MinPlayers: 2})
	host := testPlayer("Alice")
	guest := testPlayer("Bob")

	room, err := mgr.CreateRoom(host, catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if room.IsPrivate() {
		t.Fatal("new room should be public by default")
	}
	if got := mgr.PublicRooms(); len(got) != 1 || got[0] != room {
		t.Fatalf("PublicRooms() = %#v, want [%p]", got, room)
	}

	if err := room.SetPrivate(host, true); err != nil {
		t.Fatalf("SetPrivate(host, true) error = %v", err)
	}
	if !room.IsPrivate() {
		t.Fatal("room should be private after host toggle")
	}
	if got := mgr.PublicRooms(); len(got) != 0 {
		t.Fatalf("PublicRooms() = %#v, want []", got)
	}

	if _, err := mgr.JoinRoom(room.Code(), guest); err != nil {
		t.Fatalf("JoinRoom(private room) error = %v", err)
	}
	if err := room.SetPrivate(guest, false); err != ErrOnlyHostCanEdit {
		t.Fatalf("SetPrivate(guest, false) error = %v, want %v", err, ErrOnlyHostCanEdit)
	}

	if err := room.SetPrivate(host, false); err != nil {
		t.Fatalf("SetPrivate(host, false) error = %v", err)
	}
	if got := mgr.PublicRooms(); len(got) != 1 || got[0] != room {
		t.Fatalf("PublicRooms() = %#v, want [%p]", got, room)
	}

	if err := room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := room.SetPrivate(host, true); err != ErrGameInProgress {
		t.Fatalf("SetPrivate(host, true) after start error = %v, want %v", err, ErrGameInProgress)
	}
}

func TestNicknameSanitizationAndConflictSuffixing(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	host := testPlayer("Alice!!!")
	other := testPlayer("A l i c e")
	third := testPlayer("!!!")

	room, err := mgr.CreateRoom(host, catalog.DefaultDecks())
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

func TestNormalizeDecksFiltersDeduplicatesAndSorts(t *testing.T) {
	catalog := testCatalog(t)
	base := catalog.DeckByID("base")
	expansion := catalog.DeckByID("expansion")
	unknown := &deck.Deck{DeckMetadata: deck.DeckMetadata{ID: "base", Name: "Base copy"}}

	got := normalizeDecks(catalog, []*deck.Deck{expansion, nil, base, base, unknown})
	if len(got) != 2 || got[0] != base || got[1] != expansion {
		t.Fatalf("normalizeDecks() = %#v, want [%p %p]", got, base, expansion)
	}
}

func TestNormalizeDecksUsesCatalogDefaultsWhenEmpty(t *testing.T) {
	catalog := testCatalog(t)
	got := normalizeDecks(catalog, nil)
	defaults := catalog.DefaultDecks()
	if len(got) != len(defaults) {
		t.Fatalf("normalizeDecks() len = %d, want %d", len(got), len(defaults))
	}
	for i := range defaults {
		if got[i] != defaults[i] {
			t.Fatalf("normalizeDecks()[%d] = %p, want %p", i, got[i], defaults[i])
		}
	}
}

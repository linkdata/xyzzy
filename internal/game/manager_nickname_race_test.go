package game

import (
	"sync"
	"testing"
)

func TestManagerSetNicknamePublishesAfterUnlock(t *testing.T) {
	catalog := testCatalog(t)
	host := testPlayer("Alice")
	player := testPlayer("Bob")
	var calls int
	var got []any
	var managerUnlocked bool
	var roomUnlocked bool
	var playerUnlocked bool
	var manager *Manager
	var room *Room
	manager = NewManagerWithOptions(catalog, Options{
		Dirty: func(tags ...any) {
			calls++
			got = append(got, tags...)
			managerUnlocked = manager.mu.TryLock()
			if managerUnlocked {
				manager.mu.Unlock()
			}
			roomUnlocked = room.mu.TryLock()
			if roomUnlocked {
				room.mu.Unlock()
			}
			playerUnlocked = player.uiMu.TryLock()
			if playerUnlocked {
				player.uiMu.Unlock()
			}
		},
	})
	var err error
	room, err = manager.CreateRoom(host, catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if joined, joinErr := manager.JoinRoom(room.Code(), player); joinErr != nil || joined != room {
		t.Fatalf("JoinRoom() = (%v, %v), want (%v, nil)", joined, joinErr, room)
	}

	manager.SetNickname(player, " A l i c e !!! ")

	if calls != 1 {
		t.Fatalf("dirty callback calls = %d, want 1", calls)
	}
	if !managerUnlocked || !roomUnlocked || !playerUnlocked {
		t.Fatalf("dirty callback lock state = manager %t, room %t, player %t; want all unlocked", managerUnlocked, roomUnlocked, playerUnlocked)
	}
	want := map[any]bool{
		manager:                             true,
		player:                              true,
		room:                                true,
		player.NicknameField().JawsGetTag(): true,
	}
	if len(got) != len(want) {
		t.Fatalf("dirty tags = %#v, want exactly %#v", got, want)
	}
	seen := make(map[any]int, len(got))
	for _, tag := range got {
		if !want[tag] {
			t.Fatalf("unexpected dirty tag %#v in %#v", tag, got)
		}
		seen[tag]++
	}
	for tag := range want {
		if seen[tag] != 1 {
			t.Fatalf("dirty tag %#v occurs %d times in %#v, want once", tag, seen[tag], got)
		}
	}
	if got := room.NicknameFor(player); got != "Alice-2" {
		t.Fatalf("NicknameFor() = %q, want Alice-2", got)
	}
	if got := player.NicknameInputValue(); got != "Alice-2" {
		t.Fatalf("NicknameInputValue() = %q, want Alice-2", got)
	}

	manager.SetNickname(host, "Alice")
	if calls != 1 {
		t.Fatalf("dirty callback calls after unchanged save = %d, want 1", calls)
	}
}

func TestManagerSetNicknameSerializesWithMembershipChanges(t *testing.T) {
	catalog := testCatalog(t)
	manager := NewManager(catalog)
	host := testPlayer("Alice")
	room, err := manager.CreateRoom(host, catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	target := testPlayer("Player")

	for trial := 0; trial < 200; trial++ {
		manager.SetNickname(target, "Player")

		start := make(chan struct{})
		var joined *Room
		var joinErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			manager.SetNickname(target, " A l i c e !!! ")
		}()
		go func() {
			defer wg.Done()
			<-start
			joined, joinErr = manager.JoinRoom(room.Code(), target)
		}()
		close(start)
		wg.Wait()

		if joinErr != nil || joined != room {
			t.Fatalf("trial %d: JoinRoom() = (%v, %v), want (%v, nil)", trial, joined, joinErr, room)
		}
		if got := room.NicknameFor(target); got != "Alice-2" {
			t.Fatalf("trial %d: seated nickname = %q, want %q", trial, got, "Alice-2")
		}
		if got := target.NicknameInputValue(); got != "Alice-2" {
			t.Fatalf("trial %d: seated nickname input = %q, want %q", trial, got, "Alice-2")
		}

		start = make(chan struct{})
		var left *Room
		var empty bool
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			manager.SetNickname(target, " C a s e y !!! ")
		}()
		go func() {
			defer wg.Done()
			<-start
			left, empty = manager.LeaveRoom(target)
		}()
		close(start)
		wg.Wait()

		if left != room || empty {
			t.Fatalf("trial %d: LeaveRoom() = (%v, %v), want (%v, false)", trial, left, empty, room)
		}
		if got := target.Room(); got != nil {
			t.Fatalf("trial %d: Room() = %v after leave, want nil", trial, got)
		}
		if got := target.NicknameValue(); got != "Casey" {
			t.Fatalf("trial %d: standalone nickname = %q, want %q", trial, got, "Casey")
		}
		if got := target.NicknameInputValue(); got != "Casey" {
			t.Fatalf("trial %d: standalone nickname input = %q, want %q", trial, got, "Casey")
		}
	}
}

package game

import (
	"sync"
	"testing"
)

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

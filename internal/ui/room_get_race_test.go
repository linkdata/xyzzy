package ui

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/linkdata/xyzzy/internal/game"
)

type concurrentRoomGETResult struct {
	path     string
	status   int
	location string
	body     string
	err      error
}

func TestConcurrentRoomGETsRedirectLosingRequestToCurrentRoom(t *testing.T) {
	h := newLiveHarness(t)

	newPlayer := func(client *http.Client, nickname string) (result *game.Player) {
		t.Helper()
		h.getWithClient(t, client, "/")
		sess := h.sessionForClient(t, client)
		result = h.app.player(sess, nil)
		h.app.Manager.SetNickname(result, nickname)
		return
	}

	firstHost := newPlayer(h.newClient(t), "FirstHost")
	firstRoom, err := h.app.createRoom(firstHost)
	if err != nil {
		t.Fatalf("createRoom(first host) error = %v", err)
	}
	secondHost := newPlayer(h.newClient(t), "SecondHost")
	secondRoom, err := h.app.createRoom(secondHost)
	if err != nil {
		t.Fatalf("createRoom(second host) error = %v", err)
	}

	client := h.newClient(t)
	player := newPlayer(client, "Viewer")
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	paths := []string{h.app.RoomURL(firstRoom.Code()), h.app.RoomURL(secondRoom.Code())}

	for trial := 0; trial < 100; trial++ {
		if got := player.Room(); got != nil {
			t.Fatalf("trial %d: Room() before requests = %v, want nil", trial, got)
		}

		requests := make([]*http.Request, len(paths))
		for i, roomPath := range paths {
			requests[i], err = http.NewRequestWithContext(t.Context(), http.MethodGet, h.server.URL+roomPath, nil)
			if err != nil {
				t.Fatalf("trial %d: NewRequest(%q) error = %v", trial, roomPath, err)
			}
		}

		start := make(chan struct{})
		results := make(chan concurrentRoomGETResult, len(requests))
		for i, req := range requests {
			roomPath := paths[i]
			go func() {
				<-start
				result := concurrentRoomGETResult{path: roomPath}
				resp, requestErr := client.Do(req)
				if requestErr == nil {
					var body []byte
					body, requestErr = io.ReadAll(resp.Body)
					requestErr = errors.Join(requestErr, resp.Body.Close())
					result.status = resp.StatusCode
					result.location = resp.Header.Get("Location")
					result.body = string(body)
				}
				result.err = requestErr
				results <- result
			}()
		}
		close(start)

		byPath := make(map[string]concurrentRoomGETResult, len(paths))
		for range requests {
			result := <-results
			if result.err != nil {
				t.Fatalf("trial %d: GET %s error = %v", trial, result.path, result.err)
			}
			byPath[result.path] = result
		}

		current := player.Room()
		if current != firstRoom && current != secondRoom {
			t.Fatalf("trial %d: Room() = %v, want one requested room", trial, current)
		}
		currentPath := h.app.RoomURL(current.Code())
		for _, roomPath := range paths {
			result := byPath[roomPath]
			if roomPath == currentPath {
				if result.status != http.StatusOK {
					t.Fatalf("trial %d: winning GET %s status = %d, want %d", trial, roomPath, result.status, http.StatusOK)
				}
				if strings.Contains(result.body, "Not seated at this table") {
					t.Fatalf("trial %d: winning GET %s rendered an unseated page", trial, roomPath)
				}
				continue
			}
			if result.status != http.StatusSeeOther {
				t.Fatalf("trial %d: losing GET %s status = %d, want %d", trial, roomPath, result.status, http.StatusSeeOther)
			}
			if result.location != currentPath {
				t.Fatalf("trial %d: losing GET %s Location = %q, want %q", trial, roomPath, result.location, currentPath)
			}
		}

		left, empty := h.app.Manager.LeaveRoom(player)
		if left != current || empty {
			t.Fatalf("trial %d: LeaveRoom() = (%v, %v), want (%v, false)", trial, left, empty, current)
		}
	}
}

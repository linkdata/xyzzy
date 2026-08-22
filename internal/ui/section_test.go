package ui

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"testing"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/tag"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/game"
)

func TestRoomSectionContainsComparableDefinitions(t *testing.T) {
	app, _ := testApp(t)
	host := &game.Player{Nickname: "Alice", NicknameInput: "Alice"}
	room, err := app.Manager.CreateRoom(host, app.Catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	viewer := &game.Player{Nickname: "Bob", NicknameInput: "Bob"}

	tests := []struct {
		name     string
		section  roomSection
		wantName string
		wantRoom *game.Room
		wantGame bool
	}{
		{
			name: "seated sidebar",
			section: roomSection{
				App: app, Player: host, RequestedCode: room.Code(), RequestedRoom: room, Kind: roomSectionSidebar,
			},
			wantName: "room_summary_panel.html",
			wantRoom: room,
		},
		{
			name: "seated main",
			section: roomSection{
				App: app, Player: host, RequestedCode: room.Code(), RequestedRoom: room, Kind: roomSectionMain,
			},
			wantName: "room_game_lobby.html",
			wantRoom: room,
			wantGame: true,
		},
		{
			name: "unseated existing sidebar",
			section: roomSection{
				App: app, Player: viewer, RequestedCode: room.Code(), RequestedRoom: room, Kind: roomSectionSidebar,
			},
		},
		{
			name: "unseated existing main",
			section: roomSection{
				App: app, Player: viewer, RequestedCode: room.Code(), RequestedRoom: room, Kind: roomSectionMain,
			},
			wantName: "room_single_panel.html",
			wantRoom: room,
		},
		{
			name: "missing sidebar",
			section: roomSection{
				App: app, Player: viewer, RequestedCode: "MISSING", Kind: roomSectionSidebar,
			},
		},
		{
			name: "missing main",
			section: roomSection{
				App: app, Player: viewer, RequestedCode: "MISSING", Kind: roomSectionMain,
			},
			wantName: "room_single_panel.html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := tt.section.JawsContains(nil)
			second := tt.section.JawsContains(nil)
			if len(first) != len(second) {
				t.Fatalf("successive child counts = %d and %d", len(first), len(second))
			}
			if tt.wantName == "" {
				if len(first) != 0 {
					t.Fatalf("JawsContains() = %#v, want no child", first)
				}
				return
			}
			if len(first) != 1 {
				t.Fatalf("JawsContains() child count = %d, want 1", len(first))
			}
			child := first[0]
			alias := child
			if child != alias {
				t.Fatal("child definition is not reflexive")
			}
			if first[0] != second[0] {
				t.Fatalf("successive child definitions differ:\nfirst:  %#v\nsecond: %#v", first[0], second[0])
			}
			tmpl, ok := first[0].(jui.Template)
			if !ok {
				t.Fatalf("child type = %T, want ui.Template value", first[0])
			}
			if tmpl.Name != tt.wantName {
				t.Fatalf("Template.Name = %q, want %q", tmpl.Name, tt.wantName)
			}
			if typ := reflect.TypeOf(tmpl.Dot); typ == nil || !typ.Comparable() {
				t.Fatalf("Template.Dot type %v is not comparable", typ)
			}
			if tt.wantGame {
				dot, ok := tmpl.Dot.(gameTemplateDot)
				if !ok || dot.Room != tt.wantRoom {
					t.Fatalf("Template.Dot = %#v, want game dot for Room %p", tmpl.Dot, tt.wantRoom)
				}
				return
			}
			dot, ok := tmpl.Dot.(roomTemplateDot)
			if !ok || dot.Room != tt.wantRoom {
				t.Fatalf("Template.Dot = %#v, want room dot for Room %p", tmpl.Dot, tt.wantRoom)
			}
		})
	}
}

func TestRoomSectionTagsStayStableAcrossMembershipChange(t *testing.T) {
	app, _ := testApp(t)
	host := &game.Player{Nickname: "Alice", NicknameInput: "Alice"}
	room, err := app.Manager.CreateRoom(host, app.Catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	viewer := &game.Player{Nickname: "Bob", NicknameInput: "Bob"}
	main := roomSection{App: app, Player: viewer, RequestedCode: room.Code(), RequestedRoom: room, Kind: roomSectionMain}
	sidebar := roomSection{App: app, Player: viewer, RequestedCode: room.Code(), RequestedRoom: room, Kind: roomSectionSidebar}

	mainBefore := expandSectionTags(t, main.JawsGetTag())
	sidebarBefore := expandSectionTags(t, sidebar.JawsGetTag())
	before := main.JawsContains(nil)
	if len(before) != 1 || before[0].(jui.Template).Name != "room_single_panel.html" {
		t.Fatalf("unseated main child = %#v, want single-room panel", before)
	}

	if _, err := app.Manager.JoinRoom(room.Code(), viewer); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	mainAfter := expandSectionTags(t, main.JawsGetTag())
	sidebarAfter := expandSectionTags(t, sidebar.JawsGetTag())
	if !reflect.DeepEqual(mainAfter, mainBefore) {
		t.Fatalf("main tags changed with membership: before=%#v after=%#v", mainBefore, mainAfter)
	}
	if !reflect.DeepEqual(sidebarAfter, sidebarBefore) {
		t.Fatalf("sidebar tags changed with membership: before=%#v after=%#v", sidebarBefore, sidebarAfter)
	}
	if !reflect.DeepEqual(mainAfter, []any{viewer, app.Manager, room}) {
		t.Fatalf("main tags = %#v, want Player, Manager, and requested Room", mainAfter)
	}
	if !reflect.DeepEqual(sidebarAfter, []any{viewer}) {
		t.Fatalf("sidebar tags = %#v, want Player", sidebarAfter)
	}

	after := main.JawsContains(nil)
	if len(after) != 1 {
		t.Fatalf("seated main child count = %d, want 1", len(after))
	}
	tmpl, ok := after[0].(jui.Template)
	if !ok || tmpl.Name != "room_game_lobby.html" {
		t.Fatalf("seated main child = %#v, want lobby game Template", after[0])
	}
	dot, ok := tmpl.Dot.(gameTemplateDot)
	if !ok || dot.Room != room {
		t.Fatalf("seated Template.Dot = %#v, want captured Room %p", tmpl.Dot, room)
	}
}

func TestRoomGameTemplateName(t *testing.T) {
	tests := []struct {
		state game.RoomState
		want  string
	}{
		{state: game.StateLobby, want: "room_game_lobby.html"},
		{state: game.StatePlaying, want: "room_game_playing.html"},
		{state: game.StateJudging, want: "room_game_judging.html"},
		{state: game.StateReview, want: "room_game_review.html"},
		{state: game.RoomState("unknown")},
	}
	for _, tt := range tests {
		if got := roomGameTemplateName(tt.state); got != tt.want {
			t.Errorf("roomGameTemplateName(%q) = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func expandSectionTags(t *testing.T, value any) (result []any) {
	t.Helper()
	var err error
	result, err = tag.TagExpand(value)
	if err != nil {
		t.Fatalf("TagExpand() error = %v", err)
	}
	return
}

var htmlIDPattern = regexp.MustCompile(`\bid="([^"]*)"`)

func TestRenderedDocumentsHaveUniqueHTMLIDs(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	hostSession := newTestSession(t, app)
	host := app.player(hostSession, nil)
	app.Manager.SetNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	tests := []struct {
		name    string
		path    string
		cookies []*http.Cookie
	}{
		{name: "lobby", path: "/"},
		{name: "seated room", path: "/room/" + room.Code(), cookies: []*http.Cookie{hostSession.Cookie()}},
		{name: "single room", path: "/room/MISSING"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.test"+tt.path, nil)
			for _, cookie := range tt.cookies {
				req.AddCookie(cookie)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d; body=%s", tt.path, rec.Code, http.StatusOK, rec.Body.String())
			}
			assertUniqueHTMLIDs(t, rec.Body.String())
		})
	}
}

func assertUniqueHTMLIDs(t *testing.T, body string) {
	t.Helper()
	seen := make(map[string]struct{})
	for _, match := range htmlIDPattern.FindAllStringSubmatch(body, -1) {
		id := match[1]
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			t.Errorf("duplicate HTML id %q", id)
			continue
		}
		seen[id] = struct{}{}
	}
	if len(seen) == 0 {
		t.Fatal("rendered document contained no non-empty HTML ids")
	}
}

var _ jaws.Container = roomSection{}

package ui

import (
	"strings"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/jtag"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/game"
)

type templateFrame struct {
	jui.Template
}

type sectionKind string

const (
	sectionLobbySidebar sectionKind = "lobby-sidebar"
	sectionLobbyMain    sectionKind = "lobby-main"
	sectionRoomSidebar  sectionKind = "room-sidebar"
	sectionRoomMain     sectionKind = "room-main"
)

type section struct {
	App           *App
	Player        *game.Player
	RequestedCode string
	Kind          sectionKind
}

func (s *section) JawsGetTag(jtag.Context) (result any) {
	switch s.Kind {
	case sectionLobbySidebar:
		result = []any{s.Player, s.App.Manager}
		return
	case sectionRoomSidebar, sectionRoomMain:
		if room := s.currentRoom(); room != nil {
			result = []any{s.Player, room}
			return
		}
		result = []any{s.Player}
		return
	}
	result = []any{s.Player}
	return
}

func (s *section) JawsContains(*jaws.Element) (result []jaws.UI) {
	dot := templateDot{App: s.App, Player: s.Player, Room: s.currentRoom()}
	switch s.Kind {
	case sectionLobbySidebar:
		result = []jaws.UI{&templateFrame{Template: jui.NewTemplate("lobby_sidebar.html", dot)}}
		return
	case sectionLobbyMain:
		result = []jaws.UI{&templateFrame{Template: jui.NewTemplate("lobby_welcome_panel.html", dot)}}
		return
	case sectionRoomSidebar:
		if s.currentRoom() == nil {
			return
		}
		result = []jaws.UI{&templateFrame{Template: jui.NewTemplate("room_summary_panel.html", dot)}}
		return
	default:
		templateName := "room_single_panel.html"
		if s.currentRoom() != nil {
			templateName = "room_game_panel.html"
		}
		result = []jaws.UI{&templateFrame{Template: jui.NewTemplate(templateName, dot)}}
		return
	}
}

func (s *section) currentRoom() (result *game.Room) {
	if s.Player.Room == nil {
		return
	}
	if s.RequestedCode != "" && !strings.EqualFold(s.Player.Room.Code(), s.RequestedCode) {
		return
	}
	result = s.Player.Room
	return
}

func normalizeRoomCode(roomCode string) (result string) {
	result = strings.ToUpper(strings.TrimSpace(roomCode))
	return
}

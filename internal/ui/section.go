package ui

import (
	"strings"

	"github.com/linkdata/jaws"
	jtag "github.com/linkdata/jaws/lib/tag"
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

func (s *section) JawsGetTag(jtag.Context) any {
	result := []any{s.Player}
	switch s.Kind {
	case sectionLobbySidebar:
		result = append(result, s.App.Manager)
	case sectionRoomSidebar, sectionRoomMain:
		if room := s.currentRoom(); room != nil {
			result = append(result, room)
		}
	}
	return result
}

func (s *section) JawsContains(*jaws.Element) []jaws.UI {
	dot := templateDot{App: s.App, Player: s.Player, Room: s.currentRoom()}
	switch s.Kind {
	case sectionLobbySidebar:
		return []jaws.UI{&templateFrame{Template: jui.NewTemplate("lobby_sidebar.html", dot)}}
	case sectionLobbyMain:
		return []jaws.UI{&templateFrame{Template: jui.NewTemplate("lobby_welcome_panel.html", dot)}}
	case sectionRoomSidebar:
		if s.currentRoom() != nil {
			return []jaws.UI{&templateFrame{Template: jui.NewTemplate("room_summary_panel.html", dot)}}
		}
		return nil
	default:
		templateName := "room_single_panel.html"
		if s.currentRoom() != nil {
			templateName = "room_game_panel.html"
		}
		return []jaws.UI{&templateFrame{Template: jui.NewTemplate(templateName, dot)}}
	}
}

func (s *section) currentRoom() (result *game.Room) {
	if s.Player.Room != nil {
		if s.RequestedCode == "" || strings.EqualFold(s.Player.Room.Code(), s.RequestedCode) {
			result = s.Player.Room
		}
	}
	return
}

func normalizeRoomCode(roomCode string) (result string) {
	result = strings.ToUpper(strings.TrimSpace(roomCode))
	return
}

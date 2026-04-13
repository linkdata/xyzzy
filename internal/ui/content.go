package ui

import (
	"strings"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/jtag"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/game"
)

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

type templateFrame struct {
	jui.Template
}

func (s *section) JawsGetTag(jtag.Context) any {
	tags := []any{s.Player}
	switch s.Kind {
	case sectionLobbySidebar:
		tags = append(tags, s.App.Manager)
	case sectionRoomSidebar, sectionRoomMain:
		if room := s.currentRoom(); room != nil {
			tags = append(tags, room)
		}
	}
	return tags
}

func (s *section) JawsContains(*jaws.Element) []jaws.UI {
	dot := templateDot{App: s.App, Player: s.Player, Room: s.currentRoom()}
	switch s.Kind {
	case sectionLobbySidebar:
		return []jaws.UI{&templateFrame{Template: jui.NewTemplate("lobby_sidebar.html", dot)}}
	case sectionLobbyMain:
		return []jaws.UI{&templateFrame{Template: jui.NewTemplate("lobby_welcome_panel.html", dot)}}
	case sectionRoomSidebar:
		if s.currentRoom() == nil {
			return nil
		}
		return []jaws.UI{&templateFrame{Template: jui.NewTemplate("room_summary_panel.html", dot)}}
	default:
		templateName := "room_single_panel.html"
		if s.currentRoom() != nil {
			templateName = "room_game_panel.html"
		}
		return []jaws.UI{&templateFrame{Template: jui.NewTemplate(templateName, dot)}}
	}
}

func (s *section) currentRoom() *game.Room {
	if s.Player == nil || s.Player.Room == nil {
		return nil
	}
	if s.RequestedCode != "" && !strings.EqualFold(s.Player.Room.Code(), s.RequestedCode) {
		return nil
	}
	return s.Player.Room
}

func normalizeRoomCode(roomCode string) string {
	return strings.ToUpper(strings.TrimSpace(roomCode))
}

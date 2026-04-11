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

func (a *App) LobbySidebar(player *game.Player) jaws.Container {
	return &section{App: a, Player: player, Kind: sectionLobbySidebar}
}

func (a *App) LobbyMain(player *game.Player) jaws.Container {
	return &section{App: a, Player: player, Kind: sectionLobbyMain}
}

func (a *App) RoomSidebar(player *game.Player, roomCode string) jaws.Container {
	return &section{App: a, Player: player, RequestedCode: normalizeRoomCode(roomCode), Kind: sectionRoomSidebar}
}

func (a *App) RoomMain(player *game.Player, roomCode string) jaws.Container {
	return &section{App: a, Player: player, RequestedCode: normalizeRoomCode(roomCode), Kind: sectionRoomMain}
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
	switch s.Kind {
	case sectionLobbySidebar:
		return []jaws.UI{&templateFrame{Template: jui.NewTemplate("lobby_sidebar.html", s.Player)}}
	case sectionLobbyMain:
		return []jaws.UI{&templateFrame{Template: jui.NewTemplate("lobby_welcome_panel.html", s.Player)}}
	case sectionRoomSidebar:
		if s.currentRoom() == nil {
			return nil
		}
		return []jaws.UI{&templateFrame{Template: jui.NewTemplate("room_summary_panel.html", s.Player)}}
	default:
		templateName := "room_single_panel.html"
		if s.currentRoom() != nil {
			templateName = "room_game_panel.html"
		}
		return []jaws.UI{&templateFrame{Template: jui.NewTemplate(templateName, s.Player)}}
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

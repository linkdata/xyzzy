package ui

import (
	"strings"

	"github.com/linkdata/jaws"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/game"
)

type roomSectionKind uint8

const (
	roomSectionSidebar roomSectionKind = iota
	roomSectionMain
)

type roomSection struct {
	App           *App
	Player        *game.Player
	RequestedCode string
	Kind          roomSectionKind
}

func (s roomSection) JawsGetTag() any {
	if s.Kind == roomSectionSidebar {
		return s.Player
	}
	return []any{s.Player, s.App.Manager}
}

func (s roomSection) JawsContains(*jaws.Element) (result []jaws.UI) {
	root := templateDot{App: s.App, Player: s.Player}
	room := s.currentRoom()
	if s.Kind == roomSectionSidebar {
		if room != nil {
			dot := roomTemplateDot{templateDot: root, Room: room}
			result = []jaws.UI{jui.NewTemplate("div", "room_summary_panel.html", dot)}
		}
		return
	}

	if room != nil {
		roomDot := roomTemplateDot{templateDot: root, Room: room}
		dot := gameTemplateDot{roomTemplateDot: roomDot}
		result = []jaws.UI{jui.NewTemplate("div", "room_game_panel.html", dot)}
		return
	}

	dot := roomTemplateDot{
		templateDot: root,
		Room:        s.App.Manager.Room(s.RequestedCode),
	}
	result = []jaws.UI{jui.NewTemplate("div", "room_single_panel.html", dot)}
	return
}

func (s roomSection) currentRoom() (result *game.Room) {
	if s.Player != nil {
		if room := s.Player.Room(); room != nil && room.Code() == s.RequestedCode {
			result = room
		}
	}
	return
}

func normalizeRoomCode(roomCode string) (result string) {
	result = strings.ToUpper(strings.TrimSpace(roomCode))
	return
}

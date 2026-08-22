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
	RequestedRoom *game.Room
	Kind          roomSectionKind
}

func (s roomSection) JawsGetTag() any {
	if s.Kind == roomSectionSidebar {
		return s.Player
	}
	tags := []any{s.Player, s.App.Manager}
	if s.RequestedRoom != nil {
		tags = append(tags, s.RequestedRoom)
	}
	return tags
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
		dot := gameTemplateDot{templateDot: root, Room: room}
		result = []jaws.UI{jui.NewTemplate("div", roomGameTemplateName(room.State()), dot)}
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
		if room := s.Player.Room(); room != nil && room == s.RequestedRoom {
			result = room
		}
	}
	return
}

func roomGameTemplateName(state game.RoomState) (result string) {
	switch state {
	case game.StateLobby:
		result = "room_game_lobby.html"
	case game.StatePlaying:
		result = "room_game_playing.html"
	case game.StateJudging:
		result = "room_game_judging.html"
	case game.StateReview:
		result = "room_game_review.html"
	}
	return
}

func normalizeRoomCode(roomCode string) (result string) {
	result = strings.ToUpper(strings.TrimSpace(roomCode))
	return
}

package ui

import (
	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/jtag"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/game"
)

type LobbyMain struct {
	Page *LobbyPage
}

func (p *LobbyPage) Main() *LobbyMain {
	return &LobbyMain{Page: p}
}

func (m *LobbyMain) JawsGetTag(jtag.Context) any {
	return []any{m.Page, m.Page.App.Manager}
}

func (m *LobbyMain) JawsContains(*jaws.Element) []jaws.UI {
	return []jaws.UI{
		jui.NewTemplate("lobby_welcome_panel.html", &LobbyPanelVM{Page: m.Page}),
		jui.NewTemplate("lobby_actions_panel.html", &LobbyPanelVM{Page: m.Page}),
		jui.NewTemplate("lobby_rooms_panel.html", &LobbyPanelVM{Page: m.Page}),
	}
}

type LobbyPanelVM struct {
	Page *LobbyPage
}

type RoomMain struct {
	Page *RoomPage
	Room *game.Room
}

func (p *RoomPage) Main() *RoomMain {
	return &RoomMain{
		Page: p,
		Room: p.Room(),
	}
}

func (m *RoomMain) JawsGetTag(jtag.Context) any {
	tags := []any{m.Page}
	if m.Room != nil {
		tags = append(tags, m.Room)
	}
	return tags
}

func (m *RoomMain) JawsContains(*jaws.Element) []jaws.UI {
	snap := m.Page.Snapshot()
	if m.Page.Nickname() == "" {
		return []jaws.UI{
			jui.NewTemplate("room_single_panel.html", &RoomSinglePanelVM{
				Page:     m.Page,
				Snapshot: snap,
				Mode:     "nickname",
			}),
		}
	}
	if !snap.Exists {
		return []jaws.UI{
			jui.NewTemplate("room_single_panel.html", &RoomSinglePanelVM{
				Page:     m.Page,
				Snapshot: snap,
				Mode:     "missing",
			}),
		}
	}
	if !snap.InRoom {
		return []jaws.UI{
			jui.NewTemplate("room_single_panel.html", &RoomSinglePanelVM{
				Page:     m.Page,
				Snapshot: snap,
				Mode:     "unavailable",
			}),
		}
	}
	vm := &RoomPanelVM{
		Page:     m.Page,
		Snapshot: snap,
	}
	return []jaws.UI{
		jui.NewTemplate("room_summary_panel.html", vm),
		jui.NewTemplate("room_game_panel.html", vm),
	}
}

type RoomPanelVM struct {
	Page     *RoomPage
	Snapshot game.RoomView
}

type RoomSinglePanelVM struct {
	Page     *RoomPage
	Snapshot game.RoomView
	Mode     string
}

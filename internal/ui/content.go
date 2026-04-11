package ui

import (
	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/jtag"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/game"
)

type lobbySection struct {
	Page         *LobbyPage
	TemplateName string
}

type lobbyRenderData struct {
	*LobbyPage
}

func (p *LobbyPage) Sidebar() *lobbySection {
	return &lobbySection{Page: p, TemplateName: "lobby_sidebar.html"}
}

func (p *LobbyPage) Main() *lobbySection {
	return &lobbySection{Page: p, TemplateName: "lobby_welcome_panel.html"}
}

func (s *lobbySection) JawsGetTag(jtag.Context) any {
	return []any{s.Page, s.Page.App.Manager}
}

func (s *lobbySection) JawsContains(*jaws.Element) []jaws.UI {
	return []jaws.UI{
		jui.NewTemplate(s.TemplateName, &lobbyRenderData{LobbyPage: s.Page}),
	}
}

type roomSection struct {
	Page    *RoomPage
	Sidebar bool
}

func (p *RoomPage) Sidebar() *roomSection {
	return &roomSection{Page: p, Sidebar: true}
}

func (p *RoomPage) Main() *roomSection {
	return &roomSection{Page: p}
}

func (s *roomSection) JawsGetTag(jtag.Context) any {
	tags := []any{s.Page}
	if room := s.Page.Room(); room != nil {
		tags = append(tags, room)
	}
	return tags
}

func (s *roomSection) JawsContains(*jaws.Element) []jaws.UI {
	data := s.Page.RenderData()
	if s.Sidebar {
		if !data.Snapshot.InRoom {
			return nil
		}
		return []jaws.UI{
			jui.NewTemplate("room_summary_panel.html", data),
		}
	}
	templateName := "room_game_panel.html"
	if data.Mode != "game" {
		templateName = "room_single_panel.html"
	}
	return []jaws.UI{jui.NewTemplate(templateName, data)}
}

type RoomRenderData struct {
	Page     *RoomPage
	Snapshot game.RoomView
	Mode     string
}

func (p *RoomPage) RenderData() *RoomRenderData {
	snap := p.Snapshot()
	mode := "game"
	switch {
	case p.Nickname() == "":
		mode = "nickname"
	case !snap.Exists:
		mode = "missing"
	case !snap.InRoom:
		mode = "unavailable"
	}
	return &RoomRenderData{
		Page:     p,
		Snapshot: snap,
		Mode:     mode,
	}
}

package ui

import (
	"strings"
	"sync"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/xyzzy/internal/game"
)

type LobbyPage struct {
	App       *App
	Session   *jaws.Session
	mu        sync.Mutex
	NickInput string
	Alert     string
}

func NewLobbyPage(app *App, sess *jaws.Session) *LobbyPage {
	return &LobbyPage{
		App:       app,
		Session:   sess,
		NickInput: app.nickname(sess),
	}
}

func (p *LobbyPage) Nickname() string { return p.App.nickname(p.Session) }

func (p *LobbyPage) CurrentRoomCode() string {
	p.App.reconcileSession(p.Session)
	return p.App.roomCode(p.Session)
}

func (p *LobbyPage) HasCurrentRoom() bool { return p.CurrentRoomCode() != "" }

func (p *LobbyPage) CurrentRoomURL() string { return p.App.roomURL(p.CurrentRoomCode()) }

func (p *LobbyPage) RoomSummaries() []game.RoomSummary { return p.App.Manager.RoomSummaries() }

func (p *LobbyPage) NicknameField() bind.Binder[string] {
	return bind.New(&p.mu, &p.NickInput)
}

func (p *LobbyPage) SaveNameAction() bind.Binder[string] {
	label := "Save Nickname"
	return bind.New(&p.mu, &label).Clicked(func(bind bind.Binder[string], elem *jaws.Element, _ string) error {
		name := strings.TrimSpace(p.NickInput)
		if name == "" {
			p.Alert = "Enter a nickname first."
			elem.Dirty(p)
			return nil
		}
		if p.HasCurrentRoom() {
			p.Alert = "Leave your current room before changing nickname."
			elem.Dirty(p)
			return nil
		}
		p.App.setNickname(p.Session, name)
		p.NickInput = name
		elem.Request.Redirect("/")
		return nil
	})
}

func (p *LobbyPage) CreateRoomAction() bind.Binder[string] {
	label := "Create Room"
	return bind.New(&p.mu, &label).Clicked(func(bind bind.Binder[string], elem *jaws.Element, _ string) error {
		if p.HasCurrentRoom() {
			elem.Request.Redirect(p.CurrentRoomURL())
			return nil
		}
		if p.Nickname() == "" {
			name := strings.TrimSpace(p.NickInput)
			if name == "" {
				p.Alert = "Choose a nickname before creating a room."
				elem.Dirty(p)
				return nil
			}
			p.App.setNickname(p.Session, name)
		}
		room, err := p.App.createRoom(p.Session)
		if err != nil {
			p.Alert = err.Error()
			elem.Dirty(p)
			return nil
		}
		elem.Request.Redirect(p.App.roomURL(room.Code()))
		return nil
	})
}

func (p *LobbyPage) LeaveCurrentRoomAction() bind.Binder[string] {
	label := "Leave Current Room"
	return bind.New(&p.mu, &label).Clicked(func(bind bind.Binder[string], elem *jaws.Element, _ string) error {
		p.App.leaveRoom(p.Session)
		p.Alert = "You left the room."
		elem.Dirty(p)
		return nil
	})
}

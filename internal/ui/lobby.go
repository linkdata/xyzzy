package ui

import (
	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/game"
)

type saveNicknameClick struct {
	App    *App
	Player *game.Player
}

func (h saveNicknameClick) JawsClick(elem *jaws.Element, _ string) error {
	h.App.setNickname(h.Player, h.Player.NicknameInput)
	h.App.Jaws.Dirty(h.App.Manager, h.Player, h.Player.Room)
	redirectURL := "/"
	if initial := elem.Request.Initial(); initial != nil && initial.URL != nil {
		if current := initial.URL.RequestURI(); current != "" {
			redirectURL = current
		}
	}
	elem.Request.Redirect(redirectURL)
	return nil
}

func (a *App) SaveNicknameClick(player *game.Player) jaws.ClickHandler {
	return saveNicknameClick{App: a, Player: player}
}

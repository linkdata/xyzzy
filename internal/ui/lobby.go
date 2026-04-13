package ui

import (
	"github.com/linkdata/jaws"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/game"
)

func (a *App) SaveNicknameClick(player *game.Player) jaws.ClickHandler {
	return jui.Clickable("Save Nickname", func(elem *jaws.Element, name string) error {
		a.setNickname(player, player.NicknameInput)
		a.Jaws.Dirty(a.Manager, player, player.Room)
		redirectURL := "/"
		if initial := elem.Request.Initial(); initial != nil && initial.URL != nil {
			if current := initial.URL.RequestURI(); current != "" {
				redirectURL = current
			}
		}
		elem.Request.Redirect(redirectURL)
		return nil
	})
}

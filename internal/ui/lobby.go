package ui

import (
	"html"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/game"
)

type saveNicknameClick struct {
	App    *App
	Player *game.Player
}

func (h saveNicknameClick) JawsClick(elem *jaws.Element, _ string) error {
	h.App.setNickname(h.Player, h.Player.NicknameInput)
	elem.Request.Redirect("/")
	return nil
}

func (a *App) SaveNicknameClick(player *game.Player) jaws.ClickHandler {
	return saveNicknameClick{App: a, Player: player}
}

type createRoomClick struct {
	App    *App
	Player *game.Player
}

func (h createRoomClick) JawsClick(elem *jaws.Element, _ string) error {
	h.App.setNickname(h.Player, h.Player.NicknameInput)
	room, err := h.App.createRoom(h.Player)
	if err != nil {
		elem.Request.Alert("warning", html.EscapeString(err.Error()))
		return nil
	}
	elem.Request.Redirect(h.App.roomURL(room.Code()))
	return nil
}

func (a *App) CreateRoomClick(player *game.Player) jaws.ClickHandler {
	return createRoomClick{App: a, Player: player}
}

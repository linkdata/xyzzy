package ui

import (
	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/xyzzy/internal/game"
)

func (p *RoomPage) ScoreTargetSlider() bind.Binder[int] {
	value := 0
	return bind.New(&p.mu, &value).
		GetLocked(func(bind bind.Binder[int], elem *jaws.Element) int {
			snap := p.Snapshot()
			if snap.IsHost && snap.State == game.StateLobby {
				elem.RemoveAttr("disabled")
			} else {
				elem.SetAttr("disabled", "")
			}
			return snap.TargetScore
		}).
		SetLocked(func(bind bind.Binder[int], elem *jaws.Element, value int) error {
			return p.withRoomMutation(elem, func(room *game.Room) error {
				return room.SetTargetScore(p.playerID(), value)
			}, nil)
		})
}

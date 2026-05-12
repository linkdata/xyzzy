package game

import (
	"sync"
	"sync/atomic"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/xyzzy/internal/deck"
)

type Player struct {
	Session *jaws.Session

	Nickname           string
	NicknameInput      string
	Score              int
	Hand               []*deck.WhiteCard
	Submitted          []*deck.WhiteCard
	SelectedCards      []*deck.WhiteCard
	SelectedSubmission *Submission

	room atomic.Pointer[Room]
	uiMu sync.Mutex
}

// Room returns the room the player is currently seated in, or nil.
// Safe to call from any goroutine.
func (p *Player) Room() (result *Room) {
	if p != nil {
		result = p.room.Load()
	}
	return
}

// setRoom updates the player's current room. Writes are expected to happen
// while the manager and room locks are held; the atomic store ensures
// concurrent readers see a consistent value.
func (p *Player) setRoom(r *Room) {
	p.room.Store(r)
}

func (p *Player) NicknameField() bind.Binder[string] {
	return bind.New(&p.uiMu, &p.NicknameInput)
}

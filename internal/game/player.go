package game

import (
	"sync"
	"sync/atomic"

	"github.com/linkdata/jaws"
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

// NicknameValue returns the player's lobby-visible nickname.
func (p *Player) NicknameValue() (result string) {
	if p != nil {
		p.uiMu.Lock()
		result = p.Nickname
		p.uiMu.Unlock()
	}
	return
}

// NicknameInputValue returns the player's current nickname input.
func (p *Player) NicknameInputValue() (result string) {
	if p != nil {
		p.uiMu.Lock()
		result = p.NicknameInput
		p.uiMu.Unlock()
	}
	return
}

func (p *Player) setNickname(nickname string) (changed bool) {
	if p != nil {
		p.uiMu.Lock()
		if changed = p.Nickname != nickname || p.NicknameInput != nickname; changed {
			p.Nickname = nickname
			p.NicknameInput = nickname
		}
		p.uiMu.Unlock()
	}
	return
}

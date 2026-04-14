package game

import (
	"sync"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/xyzzy/internal/deck"
)

type Player struct {
	Session *jaws.Session

	Nickname           string
	NicknameInput      string
	Room               *Room
	Score              int
	Hand               []*deck.WhiteCard
	Submitted          []*deck.WhiteCard
	SelectedCards      []*deck.WhiteCard
	SelectedSubmission *Submission

	uiMu sync.Mutex
}

func (p *Player) NicknameField() bind.Binder[string] {
	return bind.New(&p.uiMu, &p.NicknameInput)
}

func (p *Player) UILocker() *sync.Mutex { return &p.uiMu }

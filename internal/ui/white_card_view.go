package ui

import (
	"strings"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type whiteCardView struct {
	Player         *game.Player
	Room           *game.Room
	Card           *deck.WhiteCard
	SelectionOrder int
}

func (v whiteCardView) WhiteFootnote() string {
	deckName := v.Room.FirstSelectedDeckNameForWhiteCard(v.Card)
	number := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, v.Card.ID)
	switch {
	case deckName == "":
		return number
	case number == "":
		return deckName
	default:
		return deckName + " · " + number
	}
}

func (d whiteCardView) JawsClick(elem *jaws.Element, name string) error {
	if d.Room.CanSubmit(d.Player) {
		if applyCardSelection(d.Player, d.Card, d.Room.NeedPick()) {
			elem.Dirty(d.Player)
		}
	}
	return nil
}

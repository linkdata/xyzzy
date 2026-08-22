package ui

import (
	"strings"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type deckInput struct {
	Room   *game.Room
	Player *game.Player
	Deck   *deck.Deck
}

type roomDeckTag struct {
	Room *game.Room
	Deck *deck.Deck
}

func (d deckInput) JawsGet(*jaws.Element) (result bool) {
	result = d.Room.DeckEnabled(d.Deck)
	return
}

func (d deckInput) JawsSet(elem *jaws.Element, value bool) (err error) {
	var changed bool
	if changed, err = d.Room.SetDeckEnabled(d.Player, d.Deck, value); err == nil {
		if !changed {
			err = jaws.ErrValueUnchanged
			return
		}
		elem.Dirty(d.Room)
	}
	return
}

func (d deckInput) JawsGetTag() (result any) {
	if d.Room != nil {
		result = roomDeckTag{Room: d.Room, Deck: d.Deck}
	}
	return
}

func cardFootnote(deckName, cardID string) (result string) {
	number := strings.Map(func(r rune) (result rune) {
		if r >= '0' && r <= '9' {
			result = r
			return
		}
		result = -1
		return
	}, cardID)
	switch {
	case deckName == "":
		result = number
	case number == "":
		result = deckName
	default:
		result = deckName + " · " + number
	}
	return
}

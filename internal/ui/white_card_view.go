package ui

import (
	"html/template"
	"strconv"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type whiteCardView struct {
	Player *game.Player
	Room   *game.Room
	Card   *deck.WhiteCard
}

func (v whiteCardView) JawsGetTag() any { return v.Room.HandCardTag(v.Player, v.Card) }

// UpdateAttrs applies selection state when the card template runs.
func (v whiteCardView) UpdateAttrs(elem *jaws.Element) string {
	elem.SetAttr("aria-pressed", strconv.FormatBool(v.SelectionOrder() > 0))
	return ""
}

func (v whiteCardView) SelectionOrder() (result int) {
	result = v.Room.SelectionOrderFor(v.Player, v.Card)
	return
}

func (v whiteCardView) WhiteFootnote() (result string) {
	result = cardFootnote(v.Room.FirstSelectedDeckNameForWhiteCard(v.Card), v.Card.ID)
	return
}

func (v whiteCardView) InitialAttrs() template.HTMLAttr {
	if v.SelectionOrder() > 0 {
		return `aria-pressed="true"`
	}
	return `aria-pressed="false"`
}

func (v whiteCardView) JawsClick(*jaws.Element, jaws.Click) error {
	v.Room.ToggleCardSelection(v.Player, v.Card)
	return nil
}

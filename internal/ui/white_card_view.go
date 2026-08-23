package ui

import (
	"html/template"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type whiteCardView struct {
	Player *game.Player
	Room   *game.Room
	Card   *deck.WhiteCard
}

// JawsGetTag returns no dependency tag.
func (whiteCardView) JawsGetTag() any { return nil }

func (v whiteCardView) SelectionOrder() (result int) {
	result = v.Room.SelectionOrderFor(v.Player, v.Card)
	return
}

func (v whiteCardView) WhiteFootnote() (result string) {
	result = cardFootnote(v.Room.FirstSelectedDeckNameForWhiteCard(v.Card), v.Card.ID)
	return
}

func (v whiteCardView) JawsInitialHTMLAttr(*jaws.Element) (result template.HTMLAttr) {
	result = cardInitialHTMLAttr(v.SelectionOrder() > 0, false, false)
	return
}

func cardInitialHTMLAttr(selected, winning, ariaDisabled bool) (result template.HTMLAttr) {
	class := `class="card-face card-face-white w-100 text-start`
	if winning {
		class += ` is-winning`
	}
	if selected {
		class += ` is-selected`
	}
	result = template.HTMLAttr(class + `"`) // #nosec G203 -- class contains only fixed application literals
	if ariaDisabled {
		result += ` aria-disabled="true"`
	}
	return
}

func (d whiteCardView) JawsClick(elem *jaws.Element, _ jaws.Click) (err error) {
	if d.Room.ToggleCardSelection(d.Player, d.Card) {
		// Wrapper attributes are initial-only, so reconstruct the cards through
		// their player-tagged parent after selection changes.
		elem.Dirty(d.Player)
	}
	return
}

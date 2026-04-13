package ui

import (
	"fmt"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/jaws/lib/jtag"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type taggedBinder[T comparable] struct {
	bind.Binder[T]
	tag any
}

func (b taggedBinder[T]) JawsGetTag(jtag.Context) any {
	return b.tag
}

type roomDeckTag struct {
	Room *game.Room
	Deck *deck.Deck
}

func applyCardSelection(player *game.Player, card *deck.WhiteCard, needPick int) (changed bool, alert string) {
	if slicesContains(player.SelectedCards, card) {
		player.SelectedCards = deleteCard(player.SelectedCards, card)
		return true, ""
	}
	if needPick == 1 {
		player.SelectedCards = []*deck.WhiteCard{card}
		return true, ""
	}
	if len(player.SelectedCards) >= needPick {
		return false, fmt.Sprintf("Select exactly %d cards.", needPick)
	}
	player.SelectedCards = append(player.SelectedCards, card)
	return true, ""
}

func slicesContains(values []*deck.WhiteCard, target *deck.WhiteCard) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func deleteCard(values []*deck.WhiteCard, target *deck.WhiteCard) []*deck.WhiteCard {
	out := values[:0]
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

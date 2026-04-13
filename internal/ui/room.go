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

func applyCardSelection(player *game.Player, cardID string, needPick int) (changed bool, alert string) {
	if slicesContains(player.SelectedCardIDs, cardID) {
		player.SelectedCardIDs = deleteString(player.SelectedCardIDs, cardID)
		return true, ""
	}
	if needPick == 1 {
		player.SelectedCardIDs = []string{cardID}
		return true, ""
	}
	if len(player.SelectedCardIDs) >= needPick {
		return false, fmt.Sprintf("Select exactly %d cards.", needPick)
	}
	player.SelectedCardIDs = append(player.SelectedCardIDs, cardID)
	return true, ""
}

func slicesContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func deleteString(values []string, target string) []string {
	out := values[:0]
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

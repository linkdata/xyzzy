package ui

import (
	"slices"

	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

func applyCardSelection(player *game.Player, card *deck.WhiteCard, needPick int) (changed bool) {
	if idx := slices.Index(player.SelectedCards, card); idx >= 0 {
		player.SelectedCards = slices.Delete(player.SelectedCards, idx, idx+1)
		changed = true
		return
	}
	if needPick == 1 {
		player.SelectedCards = []*deck.WhiteCard{card}
		changed = true
		return
	}
	if len(player.SelectedCards) >= needPick {
		changed = false
		return
	}
	player.SelectedCards = append(player.SelectedCards, card)
	changed = true
	return
}

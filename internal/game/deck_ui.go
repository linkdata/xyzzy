package game

import (
	"fmt"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/jaws/lib/ui"
)

// DeckSelectionTag identifies counts and readiness derived from selected decks.
func (r *Room) DeckSelectionTag() any { return &r.selectedDecks }

// DeckCounts returns the selected-card counts as escaped text.
func (r *Room) DeckCounts() bind.Getter[string] {
	return bind.StringGetterFunc(func(*jaws.Element) string {
		r.mu.RLock()
		black, white := r.catalog.UnionCounts(r.selectedDecks)
		r.mu.RUnlock()
		return fmt.Sprintf("%d black / %d white selected", black, white)
	}, r.DeckSelectionTag())
}

type startGameControl struct {
	ui.Button
	room   *Room
	player *Player
}

// StartGameControl keeps the action's button while its readiness changes.
func (r *Room) StartGameControl(player *Player) *startGameControl {
	return &startGameControl{Button: *ui.NewButton(r.StartGameButton(player)), room: r, player: player}
}

func (b *startGameControl) JawsUpdate(elem *jaws.Element) {
	b.room.mu.RLock()
	enabled := b.room.canStartLocked(b.player)
	b.room.mu.RUnlock()
	if enabled {
		elem.RemoveAttr("disabled")
	} else {
		elem.SetAttr("disabled", "")
	}
}

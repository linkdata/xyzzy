package game

import (
	"fmt"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
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

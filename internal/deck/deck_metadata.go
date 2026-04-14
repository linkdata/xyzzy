package deck

// DeckMetadata describes a selectable deck.
type DeckMetadata struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Weight           int    `json:"weight"`
	BaseDeck         bool   `json:"base_deck"`
	EnabledByDefault bool   `json:"enabled_by_default"`
}

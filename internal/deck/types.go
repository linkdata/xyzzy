package deck

import "html/template"

// BlackCard is a black prompt card loaded from embedded JSON.
type BlackCard struct {
	ID        string        `json:"id"`
	Text      string        `json:"text"`
	HTML      template.HTML `json:"-"`
	Pick      int           `json:"pick"`
	Draw      int           `json:"draw"`
	Watermark string        `json:"watermark,omitempty"`
}

// WhiteCard is a white answer card loaded from embedded JSON.
type WhiteCard struct {
	ID        string        `json:"id"`
	Text      string        `json:"text"`
	HTML      template.HTML `json:"-"`
	Watermark string        `json:"watermark,omitempty"`
}

// DeckMetadata describes a selectable deck.
type DeckMetadata struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Weight           int    `json:"weight"`
	BaseDeck         bool   `json:"base_deck"`
	EnabledByDefault bool   `json:"enabled_by_default"`
}

// Deck is a playable deck with its referenced card IDs.
type Deck struct {
	DeckMetadata
	BlackCards []*BlackCard
	WhiteCards []*WhiteCard
}

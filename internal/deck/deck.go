package deck

// Deck is a playable deck with its referenced card IDs.
type Deck struct {
	DeckMetadata
	BlackCards []*BlackCard
	WhiteCards []*WhiteCard
}

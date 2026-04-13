package deck

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

const (
	blackDir = "assets/cards/black"
	whiteDir = "assets/cards/white"
	decksDir = "assets/decks"
)

var (
	ErrDuplicateCardID = errors.New("duplicate card id")
	ErrDuplicateDeckID = errors.New("duplicate deck id")
	ErrInvalidDeck     = errors.New("invalid deck")
)

// Catalog is the immutable in-memory deck catalog loaded from embedded assets.
type Catalog struct {
	BlackCards map[string]*BlackCard
	WhiteCards map[string]*WhiteCard
	Decks      map[string]*Deck
	ordered    []*Deck
	defaultIDs []string
}

// LoadFS loads a catalog from the provided filesystem.
func LoadFS(fsys fs.FS) (*Catalog, error) {
	c := &Catalog{
		BlackCards: make(map[string]*BlackCard),
		WhiteCards: make(map[string]*WhiteCard),
		Decks:      make(map[string]*Deck),
	}
	if err := loadJSONDir(fsys, blackDir, func(name string, raw []byte) error {
		card := new(BlackCard)
		if err := json.Unmarshal(raw, card); err != nil {
			return fmt.Errorf("%s: decode black card: %w", name, err)
		}
		card.Pick = max(card.Pick, 1)
		if card.ID == "" || strings.TrimSpace(card.Text) == "" {
			return fmt.Errorf("%s: %w: black card missing id or text", name, ErrInvalidDeck)
		}
		card.HTML = formatCardHTML(card.Text)
		if _, exists := c.BlackCards[card.ID]; exists {
			return fmt.Errorf("%s: %w %q", name, ErrDuplicateCardID, card.ID)
		}
		c.BlackCards[card.ID] = card
		return nil
	}); err != nil {
		return nil, err
	}
	if err := loadJSONDir(fsys, whiteDir, func(name string, raw []byte) error {
		card := new(WhiteCard)
		if err := json.Unmarshal(raw, card); err != nil {
			return fmt.Errorf("%s: decode white card: %w", name, err)
		}
		if card.ID == "" || strings.TrimSpace(card.Text) == "" {
			return fmt.Errorf("%s: %w: white card missing id or text", name, ErrInvalidDeck)
		}
		card.HTML = formatCardHTML(card.Text)
		if _, exists := c.WhiteCards[card.ID]; exists {
			return fmt.Errorf("%s: %w %q", name, ErrDuplicateCardID, card.ID)
		}
		c.WhiteCards[card.ID] = card
		return nil
	}); err != nil {
		return nil, err
	}

	deckEntries, err := fs.ReadDir(fsys, decksDir)
	if err != nil {
		return nil, fmt.Errorf("read decks: %w", err)
	}
	for _, entry := range deckEntries {
		if !entry.IsDir() {
			continue
		}
		dir := path.Join(decksDir, entry.Name())
		meta, blackIDs, whiteIDs, err := loadDeckDir(fsys, dir)
		if err != nil {
			return nil, err
		}
		deck := &Deck{
			DeckMetadata: meta,
			BlackCards:   make([]*BlackCard, 0, len(blackIDs)),
			WhiteCards:   make([]*WhiteCard, 0, len(whiteIDs)),
		}
		if _, exists := c.Decks[deck.ID]; exists {
			return nil, fmt.Errorf("%s: %w %q", dir, ErrDuplicateDeckID, deck.ID)
		}
		for _, cardID := range blackIDs {
			card, ok := c.BlackCards[cardID]
			if !ok {
				return nil, fmt.Errorf("%s: unknown black card id %q", dir, cardID)
			}
			deck.BlackCards = append(deck.BlackCards, card)
		}
		for _, cardID := range whiteIDs {
			card, ok := c.WhiteCards[cardID]
			if !ok {
				return nil, fmt.Errorf("%s: unknown white card id %q", dir, cardID)
			}
			deck.WhiteCards = append(deck.WhiteCards, card)
		}
		c.Decks[deck.ID] = deck
		c.ordered = append(c.ordered, deck)
		if deck.EnabledByDefault {
			c.defaultIDs = append(c.defaultIDs, deck.ID)
		}
	}
	slices.SortFunc(c.ordered, func(a, b *Deck) int {
		if a.Weight != b.Weight {
			return a.Weight - b.Weight
		}
		return strings.Compare(a.Name, b.Name)
	})
	if len(c.defaultIDs) == 0 && len(c.ordered) > 0 {
		c.defaultIDs = []string{c.ordered[0].ID}
	}
	slices.Sort(c.defaultIDs)
	return c, nil
}

func loadDeckDir(fsys fs.FS, dir string) (DeckMetadata, []string, []string, error) {
	metaPath := path.Join(dir, "deck.json")
	raw, err := fs.ReadFile(fsys, metaPath)
	if err != nil {
		return DeckMetadata{}, nil, nil, fmt.Errorf("%s: read deck metadata: %w", metaPath, err)
	}
	var meta DeckMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return DeckMetadata{}, nil, nil, fmt.Errorf("%s: decode deck metadata: %w", metaPath, err)
	}
	if meta.ID == "" || strings.TrimSpace(meta.Name) == "" {
		return DeckMetadata{}, nil, nil, fmt.Errorf("%s: %w: deck missing id or name", metaPath, ErrInvalidDeck)
	}
	blackIDs, err := loadStringList(fsys, path.Join(dir, "black.json"))
	if err != nil {
		return DeckMetadata{}, nil, nil, err
	}
	whiteIDs, err := loadStringList(fsys, path.Join(dir, "white.json"))
	if err != nil {
		return DeckMetadata{}, nil, nil, err
	}
	blackIDs = uniqueSorted(blackIDs)
	whiteIDs = uniqueSorted(whiteIDs)
	if len(blackIDs) == 0 && len(whiteIDs) == 0 {
		return DeckMetadata{}, nil, nil, fmt.Errorf("%s: %w: deck must include at least one card", dir, ErrInvalidDeck)
	}
	return meta, blackIDs, whiteIDs, nil
}

func loadStringList(fsys fs.FS, name string) ([]string, error) {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("%s: read card membership: %w", name, err)
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, fmt.Errorf("%s: decode card membership: %w", name, err)
	}
	for i, id := range ids {
		ids[i] = strings.TrimSpace(id)
		if ids[i] == "" {
			return nil, fmt.Errorf("%s: %w: blank card id", name, ErrInvalidDeck)
		}
	}
	return ids, nil
}

func loadJSONDir(fsys fs.FS, dir string, fn func(name string, raw []byte) error) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".json" {
			continue
		}
		name := path.Join(dir, entry.Name())
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("%s: read: %w", name, err)
		}
		if err := fn(name, raw); err != nil {
			return err
		}
	}
	return nil
}

// OrderedDecks returns the deck list sorted by weight then name.
func (c *Catalog) OrderedDecks() []*Deck {
	if c == nil {
		return nil
	}
	out := make([]*Deck, len(c.ordered))
	copy(out, c.ordered)
	return out
}

// DefaultDeckIDs returns the default-selected deck IDs.
func (c *Catalog) DefaultDeckIDs() []string {
	if c == nil {
		return nil
	}
	out := make([]string, len(c.defaultIDs))
	copy(out, c.defaultIDs)
	return out
}

// UnionCounts returns the unique black and white card counts for the selected decks.
func (c *Catalog) UnionCounts(deckIDs []string) (blackCount, whiteCount int, err error) {
	blackSet, whiteSet, err := c.unionSet(deckIDs)
	if err != nil {
		return 0, 0, err
	}
	return len(blackSet), len(whiteSet), nil
}

// UnionCards returns unique cards from the selected decks, sorted by card ID.
func (c *Catalog) UnionCards(deckIDs []string) (black []*BlackCard, white []*WhiteCard, err error) {
	blackSet, whiteSet, err := c.unionSet(deckIDs)
	if err != nil {
		return nil, nil, err
	}
	for card := range blackSet {
		black = append(black, card)
	}
	for card := range whiteSet {
		white = append(white, card)
	}
	slices.SortFunc(black, func(a, b *BlackCard) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(white, func(a, b *WhiteCard) int { return strings.Compare(a.ID, b.ID) })
	return black, white, nil
}

// DeckByID returns a deck by ID.
func (c *Catalog) DeckByID(id string) *Deck {
	if c == nil {
		return nil
	}
	return c.Decks[id]
}

func (c *Catalog) unionSet(deckIDs []string) (map[*BlackCard]struct{}, map[*WhiteCard]struct{}, error) {
	if c == nil {
		return nil, nil, nil
	}
	blackSet := make(map[*BlackCard]struct{})
	whiteSet := make(map[*WhiteCard]struct{})
	for _, deckID := range uniqueSorted(deckIDs) {
		deck := c.Decks[deckID]
		if deck == nil {
			return nil, nil, fmt.Errorf("unknown deck id %q", deckID)
		}
		for _, card := range deck.BlackCards {
			if card == nil {
				continue
			}
			blackSet[card] = struct{}{}
		}
		for _, card := range deck.WhiteCards {
			if card == nil {
				continue
			}
			whiteSet[card] = struct{}{}
		}
	}
	return blackSet, whiteSet, nil
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := set[value]; ok {
			continue
		}
		set[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

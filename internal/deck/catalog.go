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
func LoadFS(fsys fs.FS) (result1 *Catalog, errResult error) {
	c := &Catalog{
		BlackCards: make(map[string]*BlackCard),
		WhiteCards: make(map[string]*WhiteCard),
		Decks:      make(map[string]*Deck),
	}
	if err := loadJSONDir(fsys, blackDir, func(name string, raw []byte) (errResult error) {
		card := new(BlackCard)
		if err := json.Unmarshal(raw, card); err != nil {
			errResult = fmt.Errorf("%s: decode black card: %w", name, err)
			return
		}
		card.Pick = max(card.Pick, 1)
		if card.ID == "" || strings.TrimSpace(card.Text) == "" {
			errResult = fmt.Errorf("%s: %w: black card missing id or text", name, ErrInvalidDeck)
			return
		}
		card.HTML = formatCardHTML(card.Text)
		if _, exists := c.BlackCards[card.ID]; exists {
			errResult = fmt.Errorf("%s: %w %q", name, ErrDuplicateCardID, card.ID)
			return
		}
		c.BlackCards[card.ID] = card
		errResult = nil
		return

	}); err != nil {
		result1, errResult = nil, err
		return
	}
	if err := loadJSONDir(fsys, whiteDir, func(name string, raw []byte) (errResult error) {
		card := new(WhiteCard)
		if err := json.Unmarshal(raw, card); err != nil {
			errResult = fmt.Errorf("%s: decode white card: %w", name, err)
			return
		}
		if card.ID == "" || strings.TrimSpace(card.Text) == "" {
			errResult = fmt.Errorf("%s: %w: white card missing id or text", name, ErrInvalidDeck)
			return
		}
		card.HTML = formatCardHTML(card.Text)
		if _, exists := c.WhiteCards[card.ID]; exists {
			errResult = fmt.Errorf("%s: %w %q", name, ErrDuplicateCardID, card.ID)
			return
		}
		c.WhiteCards[card.ID] = card
		errResult = nil
		return

	}); err != nil {
		result1, errResult = nil, err
		return
	}

	deckEntries, err := fs.ReadDir(fsys, decksDir)
	if err != nil {
		result1, errResult = nil, fmt.Errorf("read decks: %w", err)
		return
	}
	for _, entry := range deckEntries {
		if !entry.IsDir() {
			continue
		}
		dir := path.Join(decksDir, entry.Name())
		meta, blackIDs, whiteIDs, err := loadDeckDir(fsys, dir)
		if err != nil {
			result1, errResult = nil, err
			return
		}
		deck := &Deck{
			DeckMetadata: meta,
			BlackCards:   make([]*BlackCard, 0, len(blackIDs)),
			WhiteCards:   make([]*WhiteCard, 0, len(whiteIDs)),
		}
		if _, exists := c.Decks[deck.ID]; exists {
			result1, errResult = nil, fmt.Errorf("%s: %w %q", dir, ErrDuplicateDeckID, deck.ID)
			return
		}
		for _, cardID := range blackIDs {
			card, ok := c.BlackCards[cardID]
			if !ok {
				result1, errResult = nil, fmt.Errorf("%s: unknown black card id %q", dir, cardID)
				return
			}
			deck.BlackCards = append(deck.BlackCards, card)
		}
		for _, cardID := range whiteIDs {
			card, ok := c.WhiteCards[cardID]
			if !ok {
				result1, errResult = nil, fmt.Errorf("%s: unknown white card id %q", dir, cardID)
				return
			}
			deck.WhiteCards = append(deck.WhiteCards, card)
		}
		c.Decks[deck.ID] = deck
		c.ordered = append(c.ordered, deck)
		if deck.EnabledByDefault {
			c.defaultIDs = append(c.defaultIDs, deck.ID)
		}
	}
	slices.SortFunc(c.ordered, func(a, b *Deck) (result int) {
		if a.Weight != b.Weight {
			result = a.Weight - b.Weight
			return
		}
		result = strings.Compare(a.Name, b.Name)
		return

	})
	if len(c.defaultIDs) == 0 && len(c.ordered) > 0 {
		c.defaultIDs = []string{c.ordered[0].ID}
	}
	slices.Sort(c.defaultIDs)
	result1, errResult = c, nil
	return
}

func loadDeckDir(fsys fs.FS, dir string) (result1 DeckMetadata, result2 []string, result3 []string, errResult error) {
	metaPath := path.Join(dir, "deck.json")
	raw, err := fs.ReadFile(fsys, metaPath)
	if err != nil {
		result1, result2, result3, errResult = DeckMetadata{}, nil, nil, fmt.Errorf("%s: read deck metadata: %w", metaPath, err)
		return
	}
	var meta DeckMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		result1, result2, result3, errResult = DeckMetadata{}, nil, nil, fmt.Errorf("%s: decode deck metadata: %w", metaPath, err)
		return
	}
	if meta.ID == "" || strings.TrimSpace(meta.Name) == "" {
		result1, result2, result3, errResult = DeckMetadata{}, nil, nil, fmt.Errorf("%s: %w: deck missing id or name", metaPath, ErrInvalidDeck)
		return
	}
	blackIDs, err := loadStringList(fsys, path.Join(dir, "black.json"))
	if err != nil {
		result1, result2, result3, errResult = DeckMetadata{}, nil, nil, err
		return
	}
	whiteIDs, err := loadStringList(fsys, path.Join(dir, "white.json"))
	if err != nil {
		result1, result2, result3, errResult = DeckMetadata{}, nil, nil, err
		return
	}
	blackIDs = uniqueSorted(blackIDs)
	whiteIDs = uniqueSorted(whiteIDs)
	if len(blackIDs) == 0 && len(whiteIDs) == 0 {
		result1, result2, result3, errResult = DeckMetadata{}, nil, nil, fmt.Errorf("%s: %w: deck must include at least one card", dir, ErrInvalidDeck)
		return
	}
	result1, result2, result3, errResult = meta, blackIDs, whiteIDs, nil
	return
}

func loadStringList(fsys fs.FS, name string) (result1 []string, errResult error) {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		result1, errResult = nil, fmt.Errorf("%s: read card membership: %w", name, err)
		return
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		result1, errResult = nil, fmt.Errorf("%s: decode card membership: %w", name, err)
		return
	}
	for i, id := range ids {
		ids[i] = strings.TrimSpace(id)
		if ids[i] == "" {
			result1, errResult = nil, fmt.Errorf("%s: %w: blank card id", name, ErrInvalidDeck)
			return
		}
	}
	result1, errResult = ids, nil
	return
}

func loadJSONDir(fsys fs.FS, dir string, fn func(name string, raw []byte) error) (errResult error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		errResult = fmt.Errorf("read %s: %w", dir, err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".json" {
			continue
		}
		name := path.Join(dir, entry.Name())
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			errResult = fmt.Errorf("%s: read: %w", name, err)
			return
		}
		if err := fn(name, raw); err != nil {
			errResult = err
			return
		}
	}
	return
}

// OrderedDecks returns the deck list sorted by weight then name.
func (c *Catalog) OrderedDecks() (result []*Deck) {
	if c != nil {
		result = make([]*Deck, len(c.ordered))
		copy(result, c.ordered)
	}
	return
}

// DefaultDeckIDs returns the default-selected deck IDs.
func (c *Catalog) DefaultDeckIDs() (result []string) {
	if c != nil {
		result = make([]string, len(c.defaultIDs))
		copy(result, c.defaultIDs)
	}
	return
}

// UnionCounts returns the unique black and white card counts for the selected decks.
func (c *Catalog) UnionCounts(deckIDs []string) (blackCount, whiteCount int) {
	blackSet, whiteSet := c.unionSet(deckIDs)
	blackCount, whiteCount = len(blackSet), len(whiteSet)
	return
}

// UnionCards returns unique cards from the selected decks, sorted by card ID.
func (c *Catalog) UnionCards(deckIDs []string) (black []*BlackCard, white []*WhiteCard) {
	blackSet, whiteSet := c.unionSet(deckIDs)
	for card := range blackSet {
		black = append(black, card)
	}
	for card := range whiteSet {
		white = append(white, card)
	}
	slices.SortFunc(black, func(a, b *BlackCard) (result int) { result = strings.Compare(a.ID, b.ID); return })
	slices.SortFunc(white, func(a, b *WhiteCard) (result int) { result = strings.Compare(a.ID, b.ID); return })
	return
}

// DeckByID returns a deck by ID.
func (c *Catalog) DeckByID(id string) (result *Deck) {
	if c != nil {
		result = c.Decks[id]
	}
	return
}

func (c *Catalog) unionSet(deckIDs []string) (blackSet map[*BlackCard]struct{}, whiteSet map[*WhiteCard]struct{}) {
	if c != nil {
		blackSet = make(map[*BlackCard]struct{})
		whiteSet = make(map[*WhiteCard]struct{})
		for _, deckID := range uniqueSorted(deckIDs) {
			deck := c.Decks[deckID]
			if deck != nil {
				for _, card := range deck.BlackCards {
					blackSet[card] = struct{}{}
				}
				for _, card := range deck.WhiteCards {
					whiteSet[card] = struct{}{}
				}
			}
		}
	}
	return
}

func uniqueSorted(values []string) (result []string) {
	set := make(map[string]struct{}, len(values))
	result = make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := set[value]; ok {
			continue
		}
		set[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return
}

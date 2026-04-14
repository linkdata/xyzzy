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

func loadBlackCard(c *Catalog, name string, raw []byte) (err error) {
	card := new(BlackCard)
	if err = json.Unmarshal(raw, card); err == nil {
		card.Pick = max(card.Pick, 1)
		if card.ID != "" && strings.TrimSpace(card.Text) != "" {
			card.HTML = formatCardHTML(card.Text)
			if _, exists := c.BlackCards[card.ID]; exists {
				err = fmt.Errorf("%s: %w %q", name, ErrDuplicateCardID, card.ID)
				return
			}
			c.BlackCards[card.ID] = card
		}
	}
	return
}

func loadWhiteCard(c *Catalog, name string, raw []byte) (err error) {
	card := new(WhiteCard)
	if err = json.Unmarshal(raw, card); err == nil {
		if card.ID != "" && strings.TrimSpace(card.Text) != "" {
			card.HTML = formatCardHTML(card.Text)
			if _, exists := c.WhiteCards[card.ID]; exists {
				err = fmt.Errorf("%s: %w %q", name, ErrDuplicateCardID, card.ID)
				return
			}
			c.WhiteCards[card.ID] = card
		}
	}
	return
}

// LoadFS loads a catalog from the provided filesystem.
func LoadFS(fsys fs.FS) (c *Catalog, err error) {
	tmp_c := &Catalog{
		BlackCards: make(map[string]*BlackCard),
		WhiteCards: make(map[string]*WhiteCard),
		Decks:      make(map[string]*Deck),
	}

	if err = loadJSONDir(fsys, blackDir, tmp_c, loadBlackCard); err == nil {
		if err = loadJSONDir(fsys, whiteDir, tmp_c, loadWhiteCard); err == nil {
			var deckEntries []fs.DirEntry
			if deckEntries, err = fs.ReadDir(fsys, decksDir); err == nil {
				for _, entry := range deckEntries {
					if !entry.IsDir() {
						continue
					}
					dir := path.Join(decksDir, entry.Name())
					var meta DeckMetadata
					var blackIDs, whiteIDs []string
					meta, blackIDs, whiteIDs, err = loadDeckDir(fsys, dir)
					if err != nil {
						return
					}
					deck := &Deck{
						DeckMetadata: meta,
						BlackCards:   make([]*BlackCard, 0, len(blackIDs)),
						WhiteCards:   make([]*WhiteCard, 0, len(whiteIDs)),
					}
					if _, exists := tmp_c.Decks[deck.ID]; exists {
						err = fmt.Errorf("%s: %w %q", dir, ErrDuplicateDeckID, deck.ID)
						return
					}
					for _, cardID := range blackIDs {
						if card, ok := tmp_c.BlackCards[cardID]; ok {
							deck.BlackCards = append(deck.BlackCards, card)
						} else {
							err = fmt.Errorf("%s: unknown black card id %q", dir, cardID)
							return
						}
					}
					for _, cardID := range whiteIDs {
						if card, ok := tmp_c.WhiteCards[cardID]; ok {
							deck.WhiteCards = append(deck.WhiteCards, card)
						} else {
							err = fmt.Errorf("%s: unknown white card id %q", dir, cardID)
							return
						}
					}
					tmp_c.Decks[deck.ID] = deck
					tmp_c.ordered = append(tmp_c.ordered, deck)
					if deck.EnabledByDefault {
						tmp_c.defaultIDs = append(tmp_c.defaultIDs, deck.ID)
					}
				}
				slices.SortFunc(tmp_c.ordered, func(a, b *Deck) (result int) {
					if a.Weight != b.Weight {
						result = a.Weight - b.Weight
						return
					}
					result = strings.Compare(a.Name, b.Name)
					return

				})
				if len(tmp_c.defaultIDs) == 0 && len(tmp_c.ordered) > 0 {
					tmp_c.defaultIDs = []string{tmp_c.ordered[0].ID}
				}
				slices.Sort(tmp_c.defaultIDs)
				c = tmp_c
			}
		}
	}
	return
}

func loadDeckDir(fsys fs.FS, dir string) (meta DeckMetadata, blackIDs []string, whiteIDs []string, err error) {
	metaPath := path.Join(dir, "deck.json")
	var raw []byte
	if raw, err = fs.ReadFile(fsys, metaPath); err == nil {
		if err = json.Unmarshal(raw, &meta); err == nil {
			if meta.ID == "" || strings.TrimSpace(meta.Name) == "" {
				err = fmt.Errorf("%s: %w: deck missing id or name", metaPath, ErrInvalidDeck)
				return
			}
			if blackIDs, err = loadStringList(fsys, path.Join(dir, "black.json")); err == nil {
				if whiteIDs, err = loadStringList(fsys, path.Join(dir, "white.json")); err == nil {
					blackIDs = uniqueSorted(blackIDs)
					whiteIDs = uniqueSorted(whiteIDs)
					if len(blackIDs) == 0 && len(whiteIDs) == 0 {
						err = fmt.Errorf("%s: %w: deck must include at least one card", dir, ErrInvalidDeck)
						return
					}
				}
			}
		}
	}
	return
}

func loadStringList(fsys fs.FS, name string) (ids []string, err error) {
	var raw []byte
	if raw, err = fs.ReadFile(fsys, name); err == nil {
		if err = json.Unmarshal(raw, &ids); err == nil {
			for i, id := range ids {
				ids[i] = strings.TrimSpace(id)
				if ids[i] == "" {
					err = fmt.Errorf("%s: %w: blank card id", name, ErrInvalidDeck)
					return
				}
			}
		}
	}
	return
}

func loadJSONDir(fsys fs.FS, dir string, c *Catalog, fn func(c *Catalog, name string, raw []byte) error) (err error) {
	var entries []fs.DirEntry
	if entries, err = fs.ReadDir(fsys, dir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || path.Ext(entry.Name()) != ".json" {
				continue
			}
			name := path.Join(dir, entry.Name())
			var raw []byte
			if raw, err = fs.ReadFile(fsys, name); err == nil {
				err = fn(c, name, raw)
			}
			if err != nil {
				return
			}
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

func uniqueSorted(values []string) []string {
	slices.Sort(values)
	return slices.Compact(values)
}

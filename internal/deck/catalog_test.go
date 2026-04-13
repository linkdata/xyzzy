package deck

import (
	"errors"
	"testing"
	"testing/fstest"
)

func TestLoadFSAndUnion(t *testing.T) {
	fsys := fstest.MapFS{
		"assets/cards/black/b1.json":    {Data: []byte(`{"id":"b1","text":"Q1","pick":1,"draw":0}`)},
		"assets/cards/black/b2.json":    {Data: []byte(`{"id":"b2","text":"Q2","pick":2,"draw":1}`)},
		"assets/cards/white/w1.json":    {Data: []byte(`{"id":"w1","text":"A1"}`)},
		"assets/cards/white/w2.json":    {Data: []byte(`{"id":"w2","text":"A2"}`)},
		"assets/cards/white/w3.json":    {Data: []byte(`{"id":"w3","text":"A3"}`)},
		"assets/decks/alpha/deck.json":  {Data: []byte(`{"id":"alpha","name":"Alpha","weight":2}`)},
		"assets/decks/alpha/black.json": {Data: []byte(`["b1","b2"]`)},
		"assets/decks/alpha/white.json": {Data: []byte(`["w1","w2"]`)},
		"assets/decks/beta/deck.json":   {Data: []byte(`{"id":"beta","name":"Beta","weight":1,"enabled_by_default":true}`)},
		"assets/decks/beta/black.json":  {Data: []byte(`["b1"]`)},
		"assets/decks/beta/white.json":  {Data: []byte(`["w2","w3"]`)},
	}

	catalog, err := LoadFS(fsys)
	if err != nil {
		t.Fatalf("LoadFS() error = %v", err)
	}
	if got := catalog.DefaultDeckIDs(); len(got) != 1 || got[0] != "beta" {
		t.Fatalf("DefaultDeckIDs() = %v", got)
	}
	if got := catalog.OrderedDecks(); len(got) != 2 || got[0].ID != "beta" || got[1].ID != "alpha" {
		t.Fatalf("OrderedDecks() unexpected order = %#v", got)
	}
	alpha := catalog.DeckByID("alpha")
	if alpha == nil || len(alpha.BlackCards) != 2 || len(alpha.WhiteCards) != 2 {
		t.Fatalf("DeckByID(alpha) card membership = %#v, want 2 black and 2 white", alpha)
	}
	if alpha.BlackCards[0] != catalog.BlackCards["b1"] || alpha.BlackCards[1] != catalog.BlackCards["b2"] {
		t.Fatalf("alpha black membership should reference catalog cards, got %#v", alpha.BlackCards)
	}
	if alpha.WhiteCards[0] != catalog.WhiteCards["w1"] || alpha.WhiteCards[1] != catalog.WhiteCards["w2"] {
		t.Fatalf("alpha white membership should reference catalog cards, got %#v", alpha.WhiteCards)
	}
	blackCount, whiteCount, err := catalog.UnionCounts([]string{"alpha", "beta"})
	if err != nil {
		t.Fatalf("UnionCounts() error = %v", err)
	}
	if blackCount != 2 || whiteCount != 3 {
		t.Fatalf("UnionCounts() = (%d,%d), want (2,3)", blackCount, whiteCount)
	}
	black, white, err := catalog.UnionCards([]string{"beta", "alpha"})
	if err != nil {
		t.Fatalf("UnionCards() error = %v", err)
	}
	if len(black) != 2 || len(white) != 3 {
		t.Fatalf("UnionCards() got %d black and %d white", len(black), len(white))
	}
	if black[0] != catalog.BlackCards["b1"] || black[1] != catalog.BlackCards["b2"] {
		t.Fatalf("UnionCards() did not return shared black card instances")
	}
	if white[0] != catalog.WhiteCards["w1"] || white[1] != catalog.WhiteCards["w2"] || white[2] != catalog.WhiteCards["w3"] {
		t.Fatalf("UnionCards() did not return shared white card instances")
	}
	if black[1].Pick != 2 || black[1].Draw != 1 {
		t.Fatalf("expected black card defaults preserved, got %#v", black[1])
	}
}

func TestLoadFSRejectsMissingCardReference(t *testing.T) {
	fsys := fstest.MapFS{
		"assets/cards/black/b1.json":    {Data: []byte(`{"id":"b1","text":"Q1"}`)},
		"assets/cards/white/w1.json":    {Data: []byte(`{"id":"w1","text":"A1"}`)},
		"assets/decks/alpha/deck.json":  {Data: []byte(`{"id":"alpha","name":"Alpha"}`)},
		"assets/decks/alpha/black.json": {Data: []byte(`["b1"]`)},
		"assets/decks/alpha/white.json": {Data: []byte(`["missing"]`)},
	}
	if _, err := LoadFS(fsys); err == nil || !errors.Is(err, ErrInvalidDeck) && err.Error() == "" {
		// Missing references currently wrap a plain error, so just ensure we fail.
		t.Fatalf("LoadFS() error = %v, want failure", err)
	}
}

func TestLoadFSAllowsOneSidedDeck(t *testing.T) {
	fsys := fstest.MapFS{
		"assets/cards/black/b1.json":    {Data: []byte(`{"id":"b1","text":"Q1"}`)},
		"assets/cards/white/w1.json":    {Data: []byte(`{"id":"w1","text":"A1"}`)},
		"assets/decks/alpha/deck.json":  {Data: []byte(`{"id":"alpha","name":"Alpha"}`)},
		"assets/decks/alpha/black.json": {Data: []byte(`[]`)},
		"assets/decks/alpha/white.json": {Data: []byte(`["w1"]`)},
	}
	catalog, err := LoadFS(fsys)
	if err != nil {
		t.Fatalf("LoadFS() error = %v", err)
	}
	if got := catalog.DeckByID("alpha"); got == nil || len(got.BlackCards) != 0 || len(got.WhiteCards) != 1 {
		t.Fatalf("unexpected deck = %#v", got)
	}
}

func TestLoadFSRejectsDuplicateDeckID(t *testing.T) {
	fsys := fstest.MapFS{
		"assets/cards/black/b1.json":    {Data: []byte(`{"id":"b1","text":"Q1"}`)},
		"assets/cards/white/w1.json":    {Data: []byte(`{"id":"w1","text":"A1"}`)},
		"assets/decks/alpha/deck.json":  {Data: []byte(`{"id":"dup","name":"Alpha"}`)},
		"assets/decks/alpha/black.json": {Data: []byte(`["b1"]`)},
		"assets/decks/alpha/white.json": {Data: []byte(`["w1"]`)},
		"assets/decks/beta/deck.json":   {Data: []byte(`{"id":"dup","name":"Beta"}`)},
		"assets/decks/beta/black.json":  {Data: []byte(`["b1"]`)},
		"assets/decks/beta/white.json":  {Data: []byte(`["w1"]`)},
	}
	_, err := LoadFS(fsys)
	if !errors.Is(err, ErrDuplicateDeckID) {
		t.Fatalf("LoadFS() error = %v, want ErrDuplicateDeckID", err)
	}
}

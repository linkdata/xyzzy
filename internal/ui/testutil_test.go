package ui

import (
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

func testCatalog(t *testing.T) *deck.Catalog {
	t.Helper()
	fsys := fstest.MapFS{
		"assets/cards/black/b1.json":    {Data: []byte(`{"id":"b1","text":"Question?"}`)},
		"assets/cards/black/b2.json":    {Data: []byte(`{"id":"b2","text":"Another question?"}`)},
		"assets/cards/white/w1.json":    {Data: []byte(`{"id":"w1","text":"Answer 1"}`)},
		"assets/cards/white/w2.json":    {Data: []byte(`{"id":"w2","text":"Answer 2"}`)},
		"assets/cards/white/w3.json":    {Data: []byte(`{"id":"w3","text":"Answer 3"}`)},
		"assets/cards/white/w4.json":    {Data: []byte(`{"id":"w4","text":"Answer 4"}`)},
		"assets/decks/base/deck.json":   {Data: []byte(`{"id":"base","name":"Base","enabled_by_default":true}`)},
		"assets/decks/base/black.json":  {Data: []byte(`["b1"]`)},
		"assets/decks/base/white.json":  {Data: []byte(`["w1","w2","w3"]`)},
		"assets/decks/extra/deck.json":  {Data: []byte(`{"id":"extra","name":"Extra"}`)},
		"assets/decks/extra/black.json": {Data: []byte(`["b2"]`)},
		"assets/decks/extra/white.json": {Data: []byte(`["w4"]`)},
	}
	catalog, err := deck.LoadFS(fsys)
	if err != nil {
		t.Fatalf("LoadFS() error = %v", err)
	}
	return catalog
}

func testApp(t *testing.T) (*App, *http.ServeMux) {
	t.Helper()
	jw, err := jaws.New()
	if err != nil {
		t.Fatalf("jaws.New() error = %v", err)
	}
	t.Cleanup(jw.Close)
	go jw.Serve()
	catalog := testCatalog(t)
	app := New(jw, catalog, game.NewManager(catalog))
	mux := http.NewServeMux()
	if err := app.SetupRoutes(mux); err != nil {
		t.Fatalf("SetupRoutes() error = %v", err)
	}
	return app, mux
}

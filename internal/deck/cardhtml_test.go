package deck

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestFormatCardHTMLAllowsLegacyInlineFormatting(t *testing.T) {
	got := string(formatCardHTML(`Today on <i>Maury</i><br>p&lt;.05`))
	want := `Today on <i>Maury</i><br>p&lt;.05`
	if got != want {
		t.Fatalf("formatCardHTML() = %q, want %q", got, want)
	}
}

func TestFormatCardHTMLEscapesUnexpectedHTML(t *testing.T) {
	got := string(formatCardHTML(`<script>alert(1)</script><i>ok</i>`))
	want := `&lt;script&gt;alert(1)&lt;/script&gt;<i>ok</i>`
	if got != want {
		t.Fatalf("formatCardHTML() = %q, want %q", got, want)
	}
}

func TestLoadFSPrecomputesCardHTML(t *testing.T) {
	fsys := fstest.MapFS{
		"assets/cards/black/b1.json":   {Data: []byte(`{"id":"b1","text":"Prompt <i>one</i>"}`)},
		"assets/cards/white/w1.json":   {Data: []byte(`{"id":"w1","text":"Answer<br>one"}`)},
		"assets/decks/base/deck.json":  {Data: []byte(`{"id":"base","name":"Base","enabled_by_default":true}`)},
		"assets/decks/base/black.json": {Data: []byte(`["b1"]`)},
		"assets/decks/base/white.json": {Data: []byte(`["w1"]`)},
	}

	catalog, err := LoadFS(fsys)
	if err != nil {
		t.Fatalf("LoadFS() error = %v", err)
	}
	if got := string(catalog.BlackCards["b1"].HTML); !strings.Contains(got, "<i>one</i>") {
		t.Fatalf("black HTML = %q, want formatted inline markup", got)
	}
	if got := string(catalog.WhiteCards["w1"].HTML); !strings.Contains(got, "Answer<br>one") {
		t.Fatalf("white HTML = %q, want formatted line break", got)
	}
}

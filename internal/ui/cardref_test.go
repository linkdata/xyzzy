package ui

import (
	"strings"
	"testing"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/jaws/lib/jtag"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

func TestHandCardRefBinderUsesSemanticTag(t *testing.T) {
	app, _ := testApp(t)
	player := &game.Player{}
	room := &game.Room{}
	card := &deck.WhiteCard{ID: "w1", Text: "Today on <i>Maury</i>"}
	ref := HandCardRef{
		Player: player,
		Room:   room,
		Card:   card,
	}
	player.Room = room
	getter := app.HandCardHTML(player, card)

	tags := jtag.MustTagExpand(nil, getter)
	if len(tags) != 1 {
		t.Fatalf("tag count = %d, want 1", len(tags))
	}
	want := ref
	if got := tags[0]; got != want {
		t.Fatalf("tag = %#v, want %#v", got, want)
	}
}

func TestHandCardHTMLUsesTemplate(t *testing.T) {
	app, _ := testApp(t)
	catalog := app.Catalog
	player := &game.Player{Nickname: "Alice", NicknameInput: "Alice"}
	room, err := app.Manager.CreateRoom(player, catalog.DefaultDeckIDs())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	player.SelectedCardIDs = []string{"w2", "w1"}
	getter := app.HandCardHTML(player, catalog.WhiteCards["w1"])
	got := string(getter.JawsGetHTML(newCardHTMLElement(app, getter)))
	if !strings.Contains(got, `<div class="card-copy">`) {
		t.Fatalf("JawsGetHTML() = %q, want card body markup", got)
	}
	if !strings.Contains(got, "Base · 1") {
		t.Fatalf("JawsGetHTML() = %q, want user-facing deck name and numeric card id footnote", got)
	}
	if !strings.Contains(got, ">#2<") {
		t.Fatalf("JawsGetHTML() = %q, want selected-card order marker", got)
	}
	_ = room
}

func TestSubmissionRefBinderUsesSemanticTag(t *testing.T) {
	app, _ := testApp(t)
	player := &game.Player{}
	room := &game.Room{}
	submission := &game.Submission{ID: "sub-1"}
	ref := SubmissionRef{
		Player:     player,
		Room:       room,
		Submission: submission,
	}
	player.Room = room
	getter := app.SubmissionHTML(player, submission)

	tags := jtag.MustTagExpand(nil, getter)
	if len(tags) != 1 {
		t.Fatalf("tag count = %d, want 1", len(tags))
	}
	want := ref
	if got := tags[0]; got != want {
		t.Fatalf("tag = %#v, want %#v", got, want)
	}
}

func TestSubmissionHTMLUsesTemplate(t *testing.T) {
	app, _ := testApp(t)
	catalog := app.Catalog
	player := &game.Player{Nickname: "Alice", NicknameInput: "Alice"}
	room, err := app.Manager.CreateRoom(player, catalog.DefaultDeckIDs())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	submission := &game.Submission{ID: "sub-1", Player: player, CardIDs: []string{"w1", "w2"}}
	getter := app.SubmissionHTML(player, submission)
	got := string(getter.JawsGetHTML(newCardHTMLElement(app, getter)))
	if !strings.Contains(got, `submission-stack`) {
		t.Fatalf("JawsGetHTML() = %q, want stacked submission markup", got)
	}
	if !strings.Contains(got, "Base · 1") || !strings.Contains(got, "Base · 2") {
		t.Fatalf("JawsGetHTML() = %q, want rendered white card footnotes", got)
	}
	_ = room
}

func newCardHTMLElement(app *App, getter bind.HTMLGetter) *jaws.Element {
	return app.Jaws.NewRequest(nil).NewElement(jui.NewButton(getter))
}

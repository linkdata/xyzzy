package ui

import (
	"strings"
	"sync"
	"testing"

	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/jaws/lib/jtag"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

func TestHandCardRefBinderUsesSemanticTag(t *testing.T) {
	player := &game.Player{}
	room := &game.Room{}
	card := &deck.WhiteCard{ID: "w1", Text: "Today on <i>Maury</i>"}
	ref := HandCardRef{
		Player: player,
		Room:   room,
		Card:   card,
	}
	binder := bind.New(&sync.Mutex{}, &ref)

	tags := jtag.MustTagExpand(nil, binder)
	if len(tags) != 1 {
		t.Fatalf("tag count = %d, want 1", len(tags))
	}
	want := handCardTag{Player: player, Room: room, Card: card}
	if got := tags[0]; got != want {
		t.Fatalf("tag = %#v, want %#v", got, want)
	}
}

func TestHandCardRefStringIncludesDeckAndCardFootnote(t *testing.T) {
	catalog := testCatalog(t)
	manager := game.NewManager(catalog)
	player := &game.Player{Nickname: "Alice", NicknameInput: "Alice"}
	room, err := manager.CreateRoom(player, catalog.DefaultDeckIDs())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	ref := HandCardRef{
		Player: player,
		Room:   room,
		Card:   catalog.WhiteCards["w1"],
	}
	got := ref.String()
	if !strings.Contains(got, "Base · 1") {
		t.Fatalf("String() = %q, want user-facing deck name and numeric card id footnote", got)
	}
}

func TestSubmissionRefBinderUsesSemanticTag(t *testing.T) {
	player := &game.Player{}
	room := &game.Room{}
	submission := &game.Submission{ID: "sub-1"}
	ref := SubmissionRef{
		Player:       player,
		Room:         room,
		Submission:   submission,
		RenderedHTML: "A <i>formatted</i> answer",
	}
	binder := bind.New(&sync.Mutex{}, &ref)

	tags := jtag.MustTagExpand(nil, binder)
	if len(tags) != 1 {
		t.Fatalf("tag count = %d, want 1", len(tags))
	}
	want := submissionTag{Player: player, Room: room, Submission: submission}
	if got := tags[0]; got != want {
		t.Fatalf("tag = %#v, want %#v", got, want)
	}
}

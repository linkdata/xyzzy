package ui

import (
	"sync"
	"testing"

	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/jaws/lib/jtag"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

func TestHandCardRefBinderUsesSemanticTag(t *testing.T) {
	room := &game.Room{}
	card := &deck.WhiteCard{ID: "w1", Text: "Today on <i>Maury</i>"}
	ref := HandCardRef{
		Room: room,
		Card: card,
	}
	binder := bind.New(&sync.Mutex{}, &ref)

	tags := jtag.MustTagExpand(nil, binder)
	if len(tags) != 1 {
		t.Fatalf("tag count = %d, want 1", len(tags))
	}
	want := handCardTag{Room: room, Card: card}
	if got := tags[0]; got != want {
		t.Fatalf("tag = %#v, want %#v", got, want)
	}
}

func TestSubmissionRefBinderUsesSemanticTag(t *testing.T) {
	room := &game.Room{}
	submission := &game.Submission{ID: "sub-1"}
	ref := SubmissionRef{
		Room:         room,
		Submission:   submission,
		RenderedHTML: "A <i>formatted</i> answer",
	}
	binder := bind.New(&sync.Mutex{}, &ref)

	tags := jtag.MustTagExpand(nil, binder)
	if len(tags) != 1 {
		t.Fatalf("tag count = %d, want 1", len(tags))
	}
	want := submissionTag{Room: room, Submission: submission}
	if got := tags[0]; got != want {
		t.Fatalf("tag = %#v, want %#v", got, want)
	}
}

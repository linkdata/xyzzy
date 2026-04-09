package ui

import (
	"fmt"
	"strings"

	"github.com/linkdata/jaws/lib/jtag"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type handCardTag struct {
	Room *game.Room
	Card *deck.WhiteCard
}

type HandCardRef struct {
	Room *game.Room
	Card *deck.WhiteCard
}

func (r HandCardRef) JawsGetTag(jtag.Context) any {
	return handCardTag{Room: r.Room, Card: r.Card}
}

func (r HandCardRef) String() string {
	if r.Card == nil {
		return ""
	}
	return string(formatCardHTML(r.Card.Text))
}

type submissionTag struct {
	Room       *game.Room
	Submission *game.Submission
}

type SubmissionRef struct {
	Room         *game.Room
	Submission   *game.Submission
	RenderedHTML string
}

func (r SubmissionRef) JawsGetTag(jtag.Context) any {
	return submissionTag{Room: r.Room, Submission: r.Submission}
}

func (r SubmissionRef) String() string {
	return r.RenderedHTML
}

func renderSubmissionHTML(cards []*deck.WhiteCard) string {
	parts := make([]string, 0, len(cards))
	for _, card := range cards {
		if card == nil {
			continue
		}
		parts = append(parts, string(formatCardHTML(card.Text)))
	}
	return strings.Join(parts, " / ")
}

func (r SubmissionRef) GoString() string {
	if r.Submission == nil {
		return "SubmissionRef{<nil>}"
	}
	return fmt.Sprintf("SubmissionRef{%q}", r.Submission.ID)
}

package ui

import (
	"fmt"
	"html"
	"strings"

	"github.com/linkdata/jaws/lib/jtag"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type handCardTag struct {
	Player *game.Player
	Room   *game.Room
	Card   *deck.WhiteCard
}

type HandCardRef struct {
	Player *game.Player
	Room   *game.Room
	Card   *deck.WhiteCard
}

func (r HandCardRef) JawsGetTag(jtag.Context) any {
	return handCardTag{Player: r.Player, Room: r.Room, Card: r.Card}
}

func (r HandCardRef) String() string {
	if r.Card == nil {
		return ""
	}
	return renderWhiteCardHTML(r.Room, r.Card)
}

type submissionTag struct {
	Player     *game.Player
	Room       *game.Room
	Submission *game.Submission
}

type SubmissionRef struct {
	Player       *game.Player
	Room         *game.Room
	Submission   *game.Submission
	RenderedHTML string
}

func (r SubmissionRef) JawsGetTag(jtag.Context) any {
	return submissionTag{Player: r.Player, Room: r.Room, Submission: r.Submission}
}

func (r SubmissionRef) String() string {
	return r.RenderedHTML
}

func renderSubmissionHTML(room *game.Room, cards []*deck.WhiteCard) string {
	parts := make([]string, 0, len(cards))
	for _, card := range cards {
		if card == nil {
			continue
		}
		parts = append(parts, `<div class="submission-card">`+renderWhiteCardHTML(room, card)+`</div>`)
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return `<div class="submission-stack">` + strings.Join(parts, "") + `</div>`
	}
}

func (r SubmissionRef) GoString() string {
	if r.Submission == nil {
		return "SubmissionRef{<nil>}"
	}
	return fmt.Sprintf("SubmissionRef{%q}", r.Submission.ID)
}

func renderWhiteCardHTML(room *game.Room, card *deck.WhiteCard) string {
	if card == nil {
		return ""
	}
	return `<div class="card-copy">` + string(formatCardHTML(card.Text)) + `</div>` +
		`<div class="card-footnote">` + html.EscapeString(cardFootnote(room.FirstSelectedDeckNameForWhiteCard(card.ID), card.ID)) + `</div>`
}

func renderBlackCardFootnote(room *game.Room, card *deck.BlackCard) string {
	if card == nil {
		return ""
	}
	return cardFootnote(room.FirstSelectedDeckNameForBlackCard(card.ID), card.ID)
}

func cardFootnote(deckName, cardID string) string {
	number := cardIDDigits(cardID)
	if deckName == "" {
		return number
	}
	if number == "" {
		return deckName
	}
	return deckName + " · " + number
}

func cardIDDigits(cardID string) string {
	if cardID == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range cardID {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		return b.String()
	}
	return ""
}

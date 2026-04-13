package ui

import (
	"fmt"
	"strings"

	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type HandCardRef struct {
	Player *game.Player
	Room   *game.Room
	Card   *deck.WhiteCard
}

type SubmissionRef struct {
	Player     *game.Player
	Room       *game.Room
	Submission *game.Submission
}

func (r SubmissionRef) GoString() string {
	if r.Submission == nil {
		return "SubmissionRef{<nil>}"
	}
	return fmt.Sprintf("SubmissionRef{%q}", r.Submission.ID)
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

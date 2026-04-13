package ui

import (
	"fmt"

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

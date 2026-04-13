package ui

import (
	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type submissionView struct {
	Room       *game.Room
	Player     *game.Player
	Submission *game.Submission
}

func (v submissionView) Cards() []whiteCardView {
	if v.Room == nil || v.Submission == nil {
		return nil
	}
	return submissionCardViews(v.Room, v.Submission)
}

func (v submissionView) JawsClick(elem *jaws.Element, name string) error {
	if !v.Room.CanJudge(v.Player) {
		return nil
	}
	if v.Player.SelectedSubmission == v.Submission {
		v.Player.SelectedSubmission = nil
	} else {
		v.Player.SelectedSubmission = v.Submission
	}
	elem.Dirty(v.Player)
	return nil
}

func submissionCardViews(room *game.Room, submission *game.Submission) []whiteCardView {
	cards := room.SubmissionCards(submission)
	views := make([]whiteCardView, 0, len(cards))
	for _, card := range cards {
		views = append(views, whiteCardView{Room: room, Card: card})
	}
	return views
}

func selectionOrder(player *game.Player, card *deck.WhiteCard) int {
	for i, selected := range player.SelectedCards {
		if selected == card {
			return i + 1
		}
	}
	return 0
}

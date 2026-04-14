package ui

import (
	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/game"
)

type submissionView struct {
	Room       *game.Room
	Player     *game.Player
	Submission *game.Submission
}

func (v submissionView) Cards() (result []whiteCardView) {
	if v.Room == nil || v.Submission == nil {
		return
	}
	result = submissionCardViews(v.Room, v.Submission)
	return
}

func (v submissionView) JawsClick(elem *jaws.Element, name string) (errResult error) {
	if !v.Room.CanJudge(v.Player) {
		return
	}
	if v.Player.SelectedSubmission == v.Submission {
		v.Player.SelectedSubmission = nil
	} else {
		v.Player.SelectedSubmission = v.Submission
	}
	elem.Dirty(v.Player)
	return
}

func submissionCardViews(room *game.Room, submission *game.Submission) (result []whiteCardView) {
	cards := room.SubmissionCards(submission)
	result = make([]whiteCardView, 0, len(cards))
	for _, card := range cards {
		result = append(result, whiteCardView{Room: room, Card: card})
	}
	return
}

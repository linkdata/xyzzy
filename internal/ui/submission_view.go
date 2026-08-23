package ui

import (
	"html/template"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/game"
)

type submissionView struct {
	Room       *game.Room
	Player     *game.Player
	Submission *game.Submission
}

// JawsGetTag returns no dependency tag.
func (submissionView) JawsGetTag() any { return nil }

func (v submissionView) Cards() (result []whiteCardView) {
	if v.Room != nil && v.Submission != nil {
		result = submissionCardViews(v.Room, v.Submission)
	}
	return
}

func (v submissionView) JawsInitialHTMLAttr(*jaws.Element) (result template.HTMLAttr) {
	result = cardInitialHTMLAttr(
		v.Room.SubmissionSelected(v.Player, v.Submission),
		v.Room.IsWinningSubmission(v.Submission),
		!v.Room.CanJudge(v.Player),
	)
	return
}

func (v submissionView) JawsClick(elem *jaws.Element, _ jaws.Click) (err error) {
	if v.Room.ToggleSubmissionSelection(v.Player, v.Submission) {
		// Wrapper attributes are initial-only, so reconstruct the cards through
		// their player-tagged parent after selection changes.
		elem.Dirty(v.Player)
	}
	return
}

func submissionCardViews(room *game.Room, submission *game.Submission) (result []whiteCardView) {
	cards := room.SubmissionCards(submission)
	result = make([]whiteCardView, 0, len(cards))
	for _, card := range cards {
		// Submitted cards omit Player so hand-selection ordinals stay private.
		result = append(result, whiteCardView{Room: room, Card: card})
	}
	return
}

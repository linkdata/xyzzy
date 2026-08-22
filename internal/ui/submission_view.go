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

func (v submissionView) Cards() (result []whiteCardView) {
	if v.Room != nil && v.Submission != nil {
		result = submissionCardViews(v.Room, v.Submission)
	}
	return
}

func (v submissionView) JawsInitialHTMLAttr(*jaws.Element) (result template.HTMLAttr) {
	class := `class="card-face card-face-white w-100 text-start`
	if v.Room.IsWinningSubmission(v.Submission) {
		class += ` is-winning`
	}
	if v.Room.SubmissionSelected(v.Player, v.Submission) {
		class += ` is-selected`
	}
	result = template.HTMLAttr(class + `"`) // #nosec G203 -- class contains only fixed application literals
	if !v.Room.CanJudge(v.Player) {
		result += ` disabled`
	}
	return
}

func (v submissionView) JawsClick(elem *jaws.Element, _ jaws.Click) (err error) {
	if v.Room.ToggleSubmissionSelection(v.Player, v.Submission) {
		elem.Dirty(v.Player)
	}
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

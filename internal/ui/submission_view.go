package ui

import (
	"html/template"
	"strconv"

	"github.com/linkdata/jaws"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/game"
)

type submissionView struct {
	Room       *game.Room
	Player     *game.Player
	Submission *game.Submission
}

func (v submissionView) JawsGetTag() any { return v.Room.SubmissionTag(v.Player, v.Submission) }

func (v submissionView) Button() submissionButton {
	return submissionButton{jui.NewTemplate("button", "submission_clickable.html", v)}
}

type submissionButton struct{ jui.Template }

func (b submissionButton) JawsUpdate(elem *jaws.Element) {
	v := b.Dot.(submissionView)
	elem.SetAttr("aria-pressed", strconv.FormatBool(v.Room.SubmissionSelected(v.Player, v.Submission)))
}

func (v submissionView) Cards() (result []whiteCardView) {
	if v.Room != nil && v.Submission != nil {
		result = submissionCardViews(v.Room, v.Submission)
	}
	return
}

func (v submissionView) InitialAttrs() (attrs template.HTMLAttr) {
	if v.Room.CanJudge(v.Player) {
		if v.Room.SubmissionSelected(v.Player, v.Submission) {
			attrs = `aria-pressed="true"`
		} else {
			attrs = `aria-pressed="false"`
		}
	} else {
		attrs = `aria-disabled="true"`
	}
	if v.Room.IsWinningSubmission(v.Submission) {
		attrs += ` data-winning="true"`
	}
	return
}

func (v submissionView) JawsClick(*jaws.Element, jaws.Click) error {
	v.Room.ToggleSubmissionSelection(v.Player, v.Submission)
	return nil
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

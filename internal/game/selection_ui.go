package game

import (
	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/deck"
)

type handCardTag struct {
	room   *Room
	player *Player
	card   *deck.WhiteCard
}
type submissionTag struct {
	room       *Room
	player     *Player
	submission *Submission
}

// HandCardTag identifies one player's selection state for a card in this room.
func (r *Room) HandCardTag(player *Player, card *deck.WhiteCard) any {
	return handCardTag{r, player, card}
}

// SubmissionTag identifies one player's selection state for a submission.
func (r *Room) SubmissionTag(player *Player, submission *Submission) any {
	return submissionTag{r, player, submission}
}

// HandReadinessTag identifies whether the player's hand selection is complete.
func (p *Player) HandReadinessTag() any { return &p.SelectedCards }

// JudgeReadinessTag identifies whether the player has chosen a submission.
func (p *Player) JudgeReadinessTag() any { return &p.SelectedSubmission }

type selectionControl struct {
	ui.Button
	room   *Room
	player *Player
	judge  bool
}

// SubmitCardsControl retains the button while hand-selection readiness changes.
func (r *Room) SubmitCardsControl(player *Player) jaws.UI {
	return &selectionControl{Button: *ui.NewButton(r.SubmitCardsButton(player)), room: r, player: player}
}

// JudgeControl retains the button while submission-selection readiness changes.
func (r *Room) JudgeControl(player *Player) jaws.UI {
	return &selectionControl{Button: *ui.NewButton(r.JudgeButton(player)), room: r, player: player, judge: true}
}

func (b *selectionControl) JawsUpdate(elem *jaws.Element) {
	b.room.mu.RLock()
	current := b.room.playerLocked(b.player)
	enabled := false
	if current != nil {
		if b.judge {
			enabled = b.room.judgeReadyLocked(current)
		} else {
			enabled = b.room.submitReadyLocked(current)
		}
	}
	b.room.mu.RUnlock()
	if enabled {
		elem.RemoveAttr("disabled")
	} else {
		elem.SetAttr("disabled", "")
	}
}

func (r *Room) submitReadyLocked(current *Player) bool {
	return current != nil && r.canSubmitLocked(current) && len(current.SelectedCards) == r.needPickLocked()
}

func (r *Room) judgeReadyLocked(current *Player) bool {
	return current != nil && r.state == StateJudging && r.judgeLocked() == current && current.SelectedSubmission != nil
}

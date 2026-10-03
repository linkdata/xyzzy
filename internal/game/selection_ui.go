package game

import "github.com/linkdata/xyzzy/internal/deck"

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

func (r *Room) submitReadyLocked(current *Player) bool {
	return current != nil && r.canSubmitLocked(current) && len(current.SelectedCards) == r.needPickLocked()
}

func (r *Room) judgeReadyLocked(current *Player) bool {
	return current != nil && r.state == StateJudging && r.judgeLocked() == current && current.SelectedSubmission != nil
}

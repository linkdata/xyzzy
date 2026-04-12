package game

import (
	"fmt"
	"html/template"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
)

func (p *Player) NicknameField() bind.Binder[string] {
	return bind.New(&p.uiMu, &p.NicknameInput)
}

func (r *Room) ScoreTargetSlider(player *Player) bind.Binder[int] {
	return bind.New(&r.mu, &r.targetScore).
		SetLocked(func(bind bind.Binder[int], elem *jaws.Element, value int) error {
			if err := r.setTargetScoreLocked(player, value); err != nil {
				return err
			}
			return nil
		})
}

func (r *Room) PrivateToggle(player *Player) bind.Binder[bool] {
	return bind.New(&r.mu, &r.private).
		SetLocked(func(bind bind.Binder[bool], elem *jaws.Element, value bool) error {
			if err := r.setPrivateLocked(player, value); err != nil {
				return err
			}
			elem.Dirty(r.manager, r)
			return nil
		})
}

func (r *Room) PrivateToggleAttrs(player *Player) template.HTMLAttr {
	if r.host == player && r.state == StateLobby {
		return ""
	}
	return `disabled`
}

func (r *Room) ScoreTargetAttrs(player *Player) template.HTMLAttr {
	if r.host == player && r.state == StateLobby {
		return ""
	}
	return `disabled`
}

func (r *Room) StartGameAttrs(player *Player) template.HTMLAttr {
	if !r.IsHost(player) {
		return `hidden`
	}
	if !r.CanStart(player) {
		return `disabled`
	}
	return ""
}

func (r *Room) StartGameClick(player *Player) jaws.ClickHandler {
	return startGameClick{Room: r, Player: player}
}

func (r *Room) SubmitCardsAttrs(player *Player) template.HTMLAttr {
	if !r.CanSubmit(player) || len(player.SelectedCardIDs) != r.NeedPick() {
		return `disabled`
	}
	return ""
}

func (r *Room) SubmitCardsClick(player *Player) jaws.ClickHandler {
	return submitCardsClick{Room: r, Player: player}
}

func (r *Room) JudgeAttrs(player *Player) template.HTMLAttr {
	if !r.CanJudge(player) || player.SelectedSubmission == nil {
		return `disabled`
	}
	return ""
}

func (r *Room) JudgeClick(player *Player) jaws.ClickHandler {
	return judgeClick{Room: r, Player: player}
}

func (r *Room) ProceedReviewAttrs(player *Player) template.HTMLAttr {
	if !r.CanProceed(player) {
		return `hidden`
	}
	return template.HTMLAttr(fmt.Sprintf(
		`class="btn btn-primary review-countdown-button" data-review-deadline="%d" data-review-label="%s"`,
		r.ReviewDeadlineUnixMilli(),
		r.ReviewButtonBase(),
	))
}

func (r *Room) ProceedReviewClick(player *Player) jaws.ClickHandler {
	return proceedReviewClick{Room: r, Player: player}
}

type startGameClick struct {
	Room   *Room
	Player *Player
}

func (h startGameClick) JawsClick(elem *jaws.Element, _ string) error {
	if h.Room == nil {
		return nil
	}
	if err := h.Room.Start(h.Player); err != nil {
		return err
	}
	h.Player.SelectedCardIDs = nil
	h.Player.SelectedSubmission = nil
	elem.Dirty(h.Player, h.Room)
	return nil
}

type submitCardsClick struct {
	Room   *Room
	Player *Player
}

func (h submitCardsClick) JawsClick(elem *jaws.Element, _ string) error {
	if h.Room == nil {
		return nil
	}
	selected := append([]string(nil), h.Player.SelectedCardIDs...)
	if err := h.Room.PlayCards(h.Player, selected); err != nil {
		return err
	}
	h.Player.SelectedCardIDs = nil
	elem.Dirty(h.Player, h.Room)
	return nil
}

type judgeClick struct {
	Room   *Room
	Player *Player
}

func (h judgeClick) JawsClick(elem *jaws.Element, _ string) error {
	if h.Room == nil {
		return nil
	}
	selected := h.Player.SelectedSubmission
	if err := h.Room.Judge(h.Player, selected); err != nil {
		return err
	}
	h.Player.SelectedSubmission = nil
	elem.Dirty(h.Player, h.Room)
	return nil
}

type proceedReviewClick struct {
	Room   *Room
	Player *Player
}

func (h proceedReviewClick) JawsClick(elem *jaws.Element, _ string) error {
	if h.Room == nil {
		return nil
	}
	if err := h.Room.ProceedReview(h.Player); err != nil {
		return err
	}
	elem.Dirty(h.Room)
	return nil
}

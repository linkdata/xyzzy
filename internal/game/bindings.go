package game

import (
	"fmt"
	"html/template"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/deck"
)

func (p *Player) NicknameField() bind.Binder[string] {
	return bind.New(&p.uiMu, &p.NicknameInput)
}

func (r *Room) ScoreTargetSlider(player *Player) bind.Binder[int] {
	return bind.New(&r.mu, &r.targetScore).
		SetLocked(func(bind bind.Binder[int], elem *jaws.Element, value int) (err error) {
			return r.setTargetScoreLocked(player, value)
		})
}

func (r *Room) PrivateToggle(player *Player) bind.Binder[bool] {
	return bind.New(&r.mu, &r.private).
		SetLocked(func(bind bind.Binder[bool], elem *jaws.Element, value bool) (err error) {
			if err = r.setPrivateLocked(player, value); err == nil {
				elem.Dirty(r.manager, r)
			}
			return
		})
}

func (r *Room) PrivateToggleAttrs(player *Player) template.HTMLAttr {
	if r.host == player && r.state == StateLobby {
		return ""
	}
	return `disabled`
}

func (r *Room) ScoreTargetAttrs(player *Player) template.HTMLAttr {
	if r.IsHost(player) && r.state == StateLobby {
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

func (r *Room) SubmitCardsAttrs(player *Player) template.HTMLAttr {
	if !r.CanSubmit(player) || len(player.SelectedCards) != r.NeedPick() {
		return `disabled`
	}
	return ""
}

func (r *Room) SubmitCardsClick(player *Player) jaws.ClickHandler {
	return ui.Clickable("Play Selected Cards", func(elem *jaws.Element, name string) (err error) {
		selected := append([]*deck.WhiteCard(nil), player.SelectedCards...)
		if err = r.PlayCards(player, selected); err == nil {
			player.SelectedCards = nil
			elem.Dirty(player, r)
		}
		return
	})
}

func (r *Room) JudgeAttrs(player *Player) template.HTMLAttr {
	if !r.CanJudge(player) || player.SelectedSubmission == nil {
		return `disabled`
	}
	return ""
}

func (r *Room) JudgeClick(player *Player) jaws.ClickHandler {
	return ui.Clickable("Pick Winner", func(elem *jaws.Element, name string) (err error) {
		selected := player.SelectedSubmission
		if err = r.Judge(player, selected); err == nil {
			player.SelectedSubmission = nil
			elem.Dirty(player, r)
		}
		return nil
	})
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
	return ui.Clickable("", func(elem *jaws.Element, name string) (err error) {
		if err = r.ProceedReview(player); err == nil {
			elem.Dirty(r)
		}
		return
	})
}

func (r *Room) StartGameClick(player *Player) jaws.ClickHandler {
	return ui.Clickable("Start Game", func(elem *jaws.Element, name string) (err error) {
		if err = r.Start(player); err == nil {
			player.SelectedCards = nil
			player.SelectedSubmission = nil
			elem.Dirty(player, r)
		}
		return
	})
}

func (r *Room) ToggleBanClick(host *Player, target *Player) jaws.ClickHandler {
	return ui.Clickable("Toggle Ban", func(elem *jaws.Element, name string) (err error) {
		if err = r.ToggleBan(host, target); err == nil {
			elem.Dirty(r.manager, r, host, target)
		}
		return
	})
}

package game

import (
	"fmt"
	"html/template"
	"time"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/jaws/lib/ui"
)

// NicknameField binds the player's editable nickname input.
//
// The binding synchronizes access to NicknameInput and uses the field pointer as
// its update tag.
func (p *Player) NicknameField() bind.Binder[string] {
	return bind.New(&p.uiMu, &p.NicknameInput)
}

// TargetScoreBinder binds the room's target score for player.
//
// Only the host may edit the score while the room is in the lobby. Accepted
// values are clamped to [Room.MinTargetScore] through 10, and an unchanged value
// returns [jaws.ErrValueUnchanged].
func (r *Room) TargetScoreBinder(player *Player) (result bind.Binder[int]) {
	result = bind.New(&r.mu, &r.targetScore).
		SetLocked(func(prev bind.Binder[int], elem *jaws.Element, value int) (err error) {
			value = r.normalizeTargetScoreLocked(value)
			if err = r.lobbyControlErrorLocked(player); err == nil {
				err = prev.JawsSetLocked(elem, value)
			}
			return
		})
	return
}

// PrivateToggle binds whether the room is private for player.
//
// Only the host may edit privacy while the room is in the lobby. An actual
// change dirties the [Manager]; an unchanged value returns
// [jaws.ErrValueUnchanged] without dirtying it.
func (r *Room) PrivateToggle(player *Player) (result bind.Binder[bool]) {
	result = bind.New(&r.mu, &r.private).
		SetLocked(func(prev bind.Binder[bool], elem *jaws.Element, value bool) (err error) {
			if err = r.lobbyControlErrorLocked(player); err == nil {
				err = prev.JawsSetLocked(elem, value)
			}
			return
		}).
		Success(func(elem *jaws.Element) {
			// Success runs after the Binder releases r.mu.
			elem.Dirty(r.manager)
		})
	return
}

// LobbyControlAttrs returns initial attributes for player-editable lobby controls.
//
// It returns disabled unless player is the host and the Room is in the lobby.
func (r *Room) LobbyControlAttrs(player *Player) (result template.HTMLAttr) {
	// Evaluate this outside Binder.InitialHTMLAttr: bind already holds an r.mu
	// read lock while running that hook, so reacquiring it here can deadlock.
	r.mu.RLock()
	if r.lobbyControlErrorLocked(player) != nil {
		result = `disabled`
	}
	r.mu.RUnlock()
	return
}

// StartGameButton returns the start-game action for player.
//
// The action is hidden from non-hosts and disabled until the lobby has enough
// players and the selected packs provide enough cards. A successful click
// starts the Room and dirties both player and Room.
func (r *Room) StartGameButton(player *Player) (result ui.Object) {
	result = ui.New("Start Game").
		Clicked(func(obj ui.Object, elem *jaws.Element, click jaws.Click) (err error) {
			if err = r.Start(player); err == nil {
				elem.Dirty(player, r)
			}
			return
		}).
		InitialHTMLAttr(func(obj ui.Object, elem *jaws.Element) (attrs template.HTMLAttr) {
			r.mu.RLock()
			switch {
			case player == nil || r.host != player:
				attrs = `hidden`
			case !r.canStartLocked(player):
				attrs = `disabled`
			}
			r.mu.RUnlock()
			return
		})
	return
}

// SubmitCardsButton returns the selected-card submission action for player.
//
// The action is disabled until player has a complete valid selection. A
// successful click submits that selection and dirties both player and Room.
func (r *Room) SubmitCardsButton(player *Player) (result ui.Object) {
	result = ui.New("Play Selected Cards").
		Clicked(func(obj ui.Object, elem *jaws.Element, click jaws.Click) (err error) {
			if err = r.PlaySelectedCards(player); err == nil {
				elem.Dirty(player, r)
			}
			return
		}).
		InitialHTMLAttr(func(obj ui.Object, elem *jaws.Element) (attrs template.HTMLAttr) {
			r.mu.RLock()
			current := r.playerLocked(player)
			if current == nil || !r.canSubmitLocked(current) || len(current.SelectedCards) != r.needPickLocked() {
				attrs = `disabled`
			}
			r.mu.RUnlock()
			return
		})
	return
}

// JudgeButton returns the selected-submission judging action for player.
//
// The action is disabled unless player is the current judge with a selected
// submission. A successful click records the winner and dirties both player and
// Room.
func (r *Room) JudgeButton(player *Player) (result ui.Object) {
	result = ui.New("Pick Winner").
		Clicked(func(obj ui.Object, elem *jaws.Element, click jaws.Click) (err error) {
			if err = r.JudgeSelectedSubmission(player); err == nil {
				elem.Dirty(player, r)
			}
			return
		}).
		InitialHTMLAttr(func(obj ui.Object, elem *jaws.Element) (attrs template.HTMLAttr) {
			r.mu.RLock()
			current := r.playerLocked(player)
			if current == nil || r.state != StateJudging || r.judgeLocked() != current || current.SelectedSubmission == nil {
				attrs = `disabled`
			}
			r.mu.RUnlock()
			return
		})
	return
}

// ReviewRender describes the review controls for one immediate render.
type ReviewRender struct {
	Title  string              // Heading; empty outside review.
	Status bind.Getter[string] // Non-judge countdown status; nil when not needed.
	Button ui.Object           // Proceed-review action; nil when the player cannot proceed.
}

// Review returns the current review display and action for player.
//
// The title and viewer role come from one synchronized snapshot. Status and
// Button countdown text remain bound to the review deadline. Button is present
// only for the current judge, and its click revalidates the review state through
// [Room.ProceedReview]. A nil player is treated as a non-judge viewer. Outside
// review, Review returns the zero value.
func (r *Room) Review(player *Player) (result ReviewRender) {
	var active bool
	var base string
	var canProceed bool
	var deadline time.Time
	var gameWinner bool
	var winnerName string

	r.mu.RLock()
	if r.state == StateReview && r.reviewWinner != nil {
		active = true
		base = r.reviewButtonBaseLocked()
		canProceed = player != nil && r.judgeLocked() == player
		deadline = r.reviewDeadline
		gameWinner = r.reviewGameWinner
		winnerName = r.reviewWinner.Nickname
	}
	r.mu.RUnlock()

	if !active {
		return
	}
	if gameWinner {
		result.Title = fmt.Sprintf("%s won the game!", winnerName)
	} else {
		result.Title = fmt.Sprintf("%s won the round!", winnerName)
	}
	if !canProceed {
		result.Status = r.reviewCountdownGetter(deadline, base, gameWinner, false)
		return
	}

	result.Button = ui.New(r.reviewCountdownGetter(deadline, base, gameWinner, true)).
		Clicked(func(obj ui.Object, elem *jaws.Element, click jaws.Click) (err error) {
			if err = r.ProceedReview(player); err == nil {
				elem.Dirty(r)
			}
			return
		})
	return
}

func (r *Room) reviewCountdownGetter(deadline time.Time, base string, gameWinner, actionable bool) (result bind.Getter[string]) {
	result = bind.StringGetterFunc(func(*jaws.Element) string {
		return reviewCountdownText(base, gameWinner, reviewCountdown(time.Now(), deadline), actionable)
	}, &r.reviewDeadline)
	return
}

func reviewCountdown(now, deadline time.Time) (result int) {
	remaining := deadline.Sub(now)
	if !deadline.IsZero() && remaining > 0 {
		result = int((remaining + time.Second - time.Nanosecond) / time.Second)
	}
	return
}

func reviewCountdownText(base string, gameWinner bool, countdown int, actionable bool) (result string) {
	if actionable {
		result = base
		if countdown > 0 {
			result = fmt.Sprintf("%s (%d)", base, countdown)
		}
		return
	}
	unit := "seconds"
	if countdown == 1 {
		unit = "second"
	}
	switch {
	case countdown <= 0 && gameWinner:
		result = "Returning to the lobby."
	case countdown <= 0:
		result = "Advancing to the next round."
	case gameWinner:
		result = fmt.Sprintf("Returning to the lobby in %d %s.", countdown, unit)
	default:
		result = fmt.Sprintf("Next round in %d %s.", countdown, unit)
	}
	return
}

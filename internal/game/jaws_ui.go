package game

import (
	"fmt"
	"html/template"
	"time"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/deck"
)

// RoundRender is a synchronized snapshot of the current black card.
type RoundRender struct {
	BlackCard *deck.BlackCard
	DeckName  string
}

// LobbyRender is a synchronized snapshot of one lobby render.
type LobbyRender struct {
	Active         bool
	LastGameWinner string
	LastGameScores []FinalScore
	BlackCount     int
	WhiteCount     int
	RequiredWhite  int
	MinimumTarget  int
}

// HandCardRender is a synchronized snapshot of one card in a player's hand.
type HandCardRender struct {
	Card           *deck.WhiteCard
	SelectionOrder int
}

// PlayingRender is a synchronized snapshot of one playing-state render.
type PlayingRender struct {
	Active        bool
	Round         RoundRender
	CanSubmit     bool
	WaitingTitle  string
	WaitingDetail string
	Hand          []HandCardRender
}

// SubmissionRender is a synchronized snapshot of one submission's UI state.
type SubmissionRender struct {
	Submission *Submission
	Selected   bool
	Winning    bool
	Enabled    bool
}

// JudgingRender is a synchronized snapshot of one judging-state render.
type JudgingRender struct {
	Active      bool
	Round       RoundRender
	CanJudge    bool
	Title       string
	Submissions []SubmissionRender
}

// Lobby returns the current lobby display for player.
//
// The returned value contains one synchronized snapshot. Active is false when
// the Room is not in the lobby or player is not seated in it.
func (r *Room) Lobby(player *Player) (result LobbyRender) {
	r.mu.RLock()
	current := r.playerLocked(player)
	if r.state == StateLobby && current != nil {
		result.Active = true
		result.LastGameWinner = r.lastGameWinner
		result.LastGameScores = append([]FinalScore(nil), r.lastGameScores...)
		result.BlackCount, result.WhiteCount = r.catalog.UnionCounts(r.selectedDecks)
		result.RequiredWhite = MinWhiteCardsPerPlayer * max(len(r.players), 1)
		result.MinimumTarget = r.minTargetScoreLocked()
	}
	r.mu.RUnlock()
	return
}

// Playing returns the current playing-state display for player.
//
// The returned value contains one synchronized snapshot. Active is false when
// the Room is not in the playing state or player is not seated in it.
func (r *Room) Playing(player *Player) (result PlayingRender) {
	var isJudge bool
	var submitted bool

	r.mu.RLock()
	current := r.playerLocked(player)
	if r.state == StatePlaying && current != nil {
		result.Active = true
		result.Round = r.roundRenderLocked()
		result.CanSubmit = r.canSubmitLocked(current)
		isJudge = r.judgeLocked() == current
		submitted = len(current.Submitted) > 0
		if result.CanSubmit {
			result.Hand = make([]HandCardRender, 0, len(current.Hand))
			for _, card := range current.Hand {
				result.Hand = append(result.Hand, HandCardRender{
					Card:           card,
					SelectionOrder: selectionOrderLocked(current, card),
				})
			}
		}
	}
	r.mu.RUnlock()

	if result.Active && !result.CanSubmit {
		switch {
		case isJudge:
			result.WaitingTitle = "Waiting for answers"
			result.WaitingDetail = "You'll choose the winner once every answer is in."
		case submitted:
			result.WaitingTitle = "Waiting for the rest of the table"
			result.WaitingDetail = "Your cards are in."
		default:
			result.WaitingTitle = "Waiting"
		}
	}
	return
}

// Judging returns the current judging-state display for player.
//
// The returned value contains one synchronized snapshot. Active is false when
// the Room is not in the judging state or player is not seated in it.
func (r *Room) Judging(player *Player) (result JudgingRender) {
	var judgeName string

	r.mu.RLock()
	current := r.playerLocked(player)
	if r.state == StateJudging && current != nil {
		result.Active = true
		result.Round = r.roundRenderLocked()
		judge := r.judgeLocked()
		result.CanJudge = judge == current
		if judge != nil {
			judgeName = judge.Nickname
		}
		result.Submissions = r.submissionRenderLocked(current, result.CanJudge, nil)
	}
	r.mu.RUnlock()

	if result.Active {
		switch {
		case result.CanJudge:
			result.Title = "Pick the Winner"
		case judgeName != "":
			result.Title = judgeName + " is picking the winner"
		default:
			result.Title = "Waiting for the judge"
		}
	}
	return
}

// NicknameField binds the player's editable nickname input.
//
// The binding synchronizes access to NicknameInput and uses the field pointer as
// its update tag. [Manager.SetNickname] publishes the same tag after committing
// a normalized nickname.
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
// starts and dirties the Room.
func (r *Room) StartGameButton(player *Player) (result ui.Object) {
	result = ui.New("Start Game").
		Clicked(func(obj ui.Object, elem *jaws.Element, click jaws.Click) (err error) {
			if err = r.Start(player); err == nil {
				elem.Dirty(r)
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
// successful click submits that selection and dirties the Room.
func (r *Room) SubmitCardsButton(player *Player) (result ui.Object) {
	result = ui.New("Play Selected Cards").
		Clicked(func(obj ui.Object, elem *jaws.Element, click jaws.Click) (err error) {
			if err = r.PlaySelectedCards(player); err == nil {
				elem.Dirty(r)
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
// submission. A successful click records the winner and dirties the Room.
func (r *Room) JudgeButton(player *Player) (result ui.Object) {
	result = ui.New("Pick Winner").
		Clicked(func(obj ui.Object, elem *jaws.Element, click jaws.Click) (err error) {
			if err = r.JudgeSelectedSubmission(player); err == nil {
				elem.Dirty(r)
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

// ReviewRender is a synchronized snapshot of one review-state render.
type ReviewRender struct {
	Active      bool
	Round       RoundRender
	Title       string              // Heading; empty outside review.
	Status      bind.Getter[string] // Non-judge countdown status; nil when not needed.
	Button      ui.Object           // Proceed-review action; nil when the player cannot proceed.
	Submissions []SubmissionRender
}

// Review returns the current review display and action for player.
//
// The round, submissions, title, and viewer role come from one synchronized
// snapshot. Status and Button countdown text remain bound to the review
// deadline. Button is present only for the current judge, and its click
// revalidates the review state through [Room.ProceedReview]. A nil player is
// treated as a non-judge viewer. Outside review, Review returns the zero value.
func (r *Room) Review(player *Player) (result ReviewRender) {
	var canProceed bool
	var deadline time.Time
	var gameWinner bool
	var winnerName string

	r.mu.RLock()
	if r.state == StateReview && r.reviewWinner != nil {
		result.Active = true
		result.Round = r.roundRenderLocked()
		canProceed = player != nil && r.judgeLocked() == player
		deadline = r.reviewDeadline
		gameWinner = r.reviewGameWinner
		winnerName = r.reviewWinner.Nickname
		result.Submissions = r.submissionRenderLocked(r.playerLocked(player), false, r.reviewSubmission)
	}
	r.mu.RUnlock()

	if !result.Active {
		return
	}
	if gameWinner {
		result.Title = fmt.Sprintf("%s won the game!", winnerName)
	} else {
		result.Title = fmt.Sprintf("%s won the round!", winnerName)
	}
	if !canProceed {
		result.Status = r.reviewCountdownGetter(deadline, gameWinner, false)
		return
	}

	result.Button = ui.New(r.reviewCountdownGetter(deadline, gameWinner, true)).
		Clicked(func(obj ui.Object, elem *jaws.Element, click jaws.Click) (err error) {
			if err = r.ProceedReview(player); err == nil {
				elem.Dirty(r)
			}
			return
		})
	return
}

func (r *Room) roundRenderLocked() (result RoundRender) {
	if black := r.currentBlackLocked(); black != nil {
		result.BlackCard = black
		result.DeckName = r.firstSelectedDeckNameForBlackCardLocked(black)
	}
	return
}

func (r *Room) submissionRenderLocked(player *Player, enabled bool, winner *Submission) (result []SubmissionRender) {
	var selected *Submission
	if player != nil {
		selected = player.SelectedSubmission
	}
	result = make([]SubmissionRender, 0, len(r.submissions))
	for _, submission := range r.submissions {
		result = append(result, SubmissionRender{
			Submission: submission,
			Selected:   submission != nil && submission == selected,
			Winning:    submission != nil && submission == winner,
			Enabled:    enabled,
		})
	}
	return
}

func (r *Room) reviewCountdownGetter(deadline time.Time, gameWinner, actionable bool) (result bind.Getter[string]) {
	result = bind.StringGetterFunc(func(*jaws.Element) string {
		return reviewCountdownText(gameWinner, reviewCountdown(time.Now(), deadline), actionable)
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

func reviewCountdownText(gameWinner bool, countdown int, actionable bool) (result string) {
	if actionable {
		result = "Next Round"
		if gameWinner {
			result = "Back to Lobby"
		}
		if countdown > 0 {
			result = fmt.Sprintf("%s (%d)", result, countdown)
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

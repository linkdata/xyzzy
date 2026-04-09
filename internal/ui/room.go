package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type RoomPage struct {
	App                *App
	Session            *jaws.Session
	RoomCode           string
	mu                 sync.Mutex
	NickInput          string
	Alert              string
	SelectedCardIDs    []string
	SelectedSubmission string
	scoreTarget        int
}

func NewRoomPage(app *App, sess *jaws.Session, roomCode string) *RoomPage {
	return &RoomPage{
		App:       app,
		Session:   sess,
		RoomCode:  strings.ToUpper(strings.TrimSpace(roomCode)),
		NickInput: app.nickname(sess),
	}
}

func (p *RoomPage) Nickname() string { return p.App.nickname(p.Session) }

func (p *RoomPage) Room() *game.Room { return p.App.Manager.GetRoom(p.RoomCode) }

func (p *RoomPage) Snapshot() game.RoomView {
	if room := p.Room(); room != nil {
		p.App.reconcileSession(p.Session)
		return room.Snapshot(p.App.playerID(p.Session))
	}
	return game.RoomView{Code: p.RoomCode}
}

func (p *RoomPage) NicknameField() bind.Binder[string] {
	return bind.New(&p.mu, &p.NickInput)
}

func (p *RoomPage) SaveNameAndJoinAction() bind.Binder[string] {
	label := "Save Nickname and Join"
	return bind.New(&p.mu, &label).Clicked(func(bind bind.Binder[string], elem *jaws.Element, _ string) error {
		name := strings.TrimSpace(p.NickInput)
		if name == "" {
			p.Alert = "Enter a nickname first."
			elem.Dirty(p)
			return nil
		}
		p.App.setNickname(p.Session, name)
		if room, err := p.App.joinRoom(p.Session, p.RoomCode); err == nil {
			elem.Request.Redirect(p.App.roomURL(room.Code()))
			return nil
		} else {
			p.Alert = err.Error()
		}
		elem.Dirty(p)
		return nil
	})
}

func (p *RoomPage) StartGameAction() bind.Binder[string] {
	label := "Start Game"
	return bind.New(&p.mu, &label).
		GetLocked(func(bind bind.Binder[string], elem *jaws.Element) string {
			snap := p.Snapshot()
			if !snap.IsHost {
				elem.SetAttr("hidden", "")
			} else if !snap.CanStart {
				elem.RemoveAttr("hidden")
				elem.SetAttr("disabled", "")
			} else {
				elem.RemoveAttr("hidden")
				elem.RemoveAttr("disabled")
			}
			return label
		}).
		Clicked(func(bind bind.Binder[string], elem *jaws.Element, _ string) error {
			room := p.Room()
			if room == nil {
				p.Alert = "Room not found."
				elem.Dirty(p)
				return nil
			}
			if err := room.Start(p.App.playerID(p.Session)); err != nil {
				p.Alert = err.Error()
				elem.Dirty(p)
				return nil
			}
			p.SelectedCardIDs = nil
			p.SelectedSubmission = ""
			p.Alert = ""
			p.App.DirtyRoom(room)
			elem.Dirty(p)
			return nil
		})
}

func (p *RoomPage) LeaveRoomAction() bind.Binder[string] {
	label := "Leave Room"
	return bind.New(&p.mu, &label).Clicked(func(bind bind.Binder[string], elem *jaws.Element, _ string) error {
		p.App.leaveRoom(p.Session)
		elem.Request.Redirect("/")
		return nil
	})
}

func (p *RoomPage) DeckToggle(deckID string) bind.Binder[bool] {
	checked := false
	return bind.New(&p.mu, &checked).
		GetLocked(func(bind bind.Binder[bool], elem *jaws.Element) bool {
			snap := p.Snapshot()
			if !snap.IsHost || snap.State != game.StateLobby {
				elem.SetAttr("disabled", "")
			} else {
				elem.RemoveAttr("disabled")
			}
			return slicesContains(snap.SelectedDeckIDs, deckID)
		}).
		SetLocked(func(bind bind.Binder[bool], elem *jaws.Element, value bool) error {
			room := p.Room()
			if room == nil {
				p.Alert = "Room not found."
				elem.Dirty(p)
				return nil
			}
			if err := room.ToggleDeck(p.App.playerID(p.Session), deckID, value); err != nil {
				p.Alert = err.Error()
				elem.Dirty(p)
				return nil
			}
			p.Alert = ""
			p.App.DirtyRoom(room)
			elem.Dirty(p)
			return nil
		})
}

func (p *RoomPage) CardAction(card *deck.WhiteCard) bind.Binder[HandCardRef] {
	ref := HandCardRef{Room: p.Room(), Card: card}
	return bind.New(&p.mu, &ref).
		GetLocked(func(bind bind.Binder[HandCardRef], elem *jaws.Element) HandCardRef {
			snap := p.Snapshot()
			if !snap.CanSubmit {
				elem.SetAttr("disabled", "")
			} else {
				elem.RemoveAttr("disabled")
			}
			if card != nil && slicesContains(p.SelectedCardIDs, card.ID) {
				elem.SetClass("is-selected")
			} else {
				elem.RemoveClass("is-selected")
			}
			return ref
		}).
		Clicked(func(bind bind.Binder[HandCardRef], elem *jaws.Element, _ string) error {
			snap := p.Snapshot()
			if !snap.CanSubmit {
				return nil
			}
			card := bind.JawsGet(elem).Card
			if card == nil {
				return nil
			}
			p.applyCardSelection(card.ID, snap.NeedPick)
			elem.Dirty(p)
			return nil
		})
}

func (p *RoomPage) SubmitCardsAction() bind.Binder[string] {
	label := "Play Selected Cards"
	return bind.New(&p.mu, &label).
		GetLocked(func(bind bind.Binder[string], elem *jaws.Element) string {
			snap := p.Snapshot()
			if !snap.CanSubmit || len(p.SelectedCardIDs) != snap.NeedPick {
				elem.SetAttr("disabled", "")
			} else {
				elem.RemoveAttr("disabled")
			}
			return label
		}).
		Clicked(func(bind bind.Binder[string], elem *jaws.Element, _ string) error {
			room := p.Room()
			if room == nil {
				p.Alert = "Room not found."
				elem.Dirty(p)
				return nil
			}
			if err := room.PlayCards(p.App.playerID(p.Session), append([]string(nil), p.SelectedCardIDs...)); err != nil {
				p.Alert = err.Error()
				elem.Dirty(p)
				return nil
			}
			p.SelectedCardIDs = nil
			p.Alert = ""
			p.App.DirtyRoom(room)
			elem.Dirty(p)
			return nil
		})
}

func (p *RoomPage) SubmissionAction(sub game.SubmissionView) bind.Binder[SubmissionRef] {
	ref := SubmissionRef{
		Room:         p.Room(),
		Submission:   sub.Submission,
		RenderedHTML: renderSubmissionHTML(p.Room(), sub.Cards),
	}
	return bind.New(&p.mu, &ref).
		GetLocked(func(bind bind.Binder[SubmissionRef], elem *jaws.Element) SubmissionRef {
			snap := p.Snapshot()
			if !snap.CanJudge {
				elem.SetAttr("disabled", "")
			} else {
				elem.RemoveAttr("disabled")
			}
			if p.SelectedSubmission == sub.ID {
				elem.SetClass("is-selected")
			} else {
				elem.RemoveClass("is-selected")
			}
			return ref
		}).
		Clicked(func(bind bind.Binder[SubmissionRef], elem *jaws.Element, _ string) error {
			if !p.Snapshot().CanJudge {
				return nil
			}
			submission := bind.JawsGet(elem).Submission
			if submission == nil {
				return nil
			}
			submissionID := submission.ID
			if p.SelectedSubmission == submissionID {
				p.SelectedSubmission = ""
			} else {
				p.SelectedSubmission = submissionID
			}
			elem.Dirty(p)
			return nil
		})
}

func (p *RoomPage) BlackCardFootnote(card *deck.BlackCard) string {
	return renderBlackCardFootnote(p.Room(), card)
}

func (p *RoomPage) WaitingTitle(snap game.RoomView) string {
	return waitingTitle(snap, p.App.playerID(p.Session))
}

func (p *RoomPage) WaitingDetail(snap game.RoomView) string {
	return waitingDetail(snap, p.App.playerID(p.Session))
}

func (p *RoomPage) applyCardSelection(cardID string, needPick int) {
	if slicesContains(p.SelectedCardIDs, cardID) {
		p.SelectedCardIDs = deleteString(p.SelectedCardIDs, cardID)
		return
	}
	if needPick == 1 {
		p.SelectedCardIDs = []string{cardID}
		p.Alert = ""
		return
	}
	if len(p.SelectedCardIDs) >= needPick {
		p.Alert = fmt.Sprintf("Select exactly %d cards.", needPick)
		return
	}
	p.SelectedCardIDs = append(p.SelectedCardIDs, cardID)
	p.Alert = ""
}

func waitingTitle(snap game.RoomView, playerID string) string {
	switch snap.State {
	case game.StateJudging:
		if snap.JudgeName != "" {
			return snap.JudgeName + " is picking the winner"
		}
		return "Waiting for the judge"
	case game.StatePlaying:
		if player := currentPlayerView(snap, playerID); player != nil && player.IsJudge {
			return "Waiting for answers"
		}
		return "Waiting for the rest of the table"
	default:
		return "Waiting"
	}
}

func waitingDetail(snap game.RoomView, playerID string) string {
	if snap.State != game.StatePlaying {
		return ""
	}
	player := currentPlayerView(snap, playerID)
	if player == nil {
		return ""
	}
	if player.IsJudge {
		return "You'll choose the winner once every answer is in."
	}
	if player.Submitted {
		return "Your cards are in."
	}
	return ""
}

func currentPlayerView(snap game.RoomView, playerID string) *game.PlayerView {
	if playerID == "" {
		return nil
	}
	for i := range snap.Players {
		if snap.Players[i].ID == playerID {
			return &snap.Players[i]
		}
	}
	return nil
}

func (p *RoomPage) JudgeAction() bind.Binder[string] {
	label := "Pick Winner"
	return bind.New(&p.mu, &label).
		GetLocked(func(bind bind.Binder[string], elem *jaws.Element) string {
			if !p.Snapshot().CanJudge || p.SelectedSubmission == "" {
				elem.SetAttr("disabled", "")
			} else {
				elem.RemoveAttr("disabled")
			}
			return label
		}).
		Clicked(func(bind bind.Binder[string], elem *jaws.Element, _ string) error {
			room := p.Room()
			if room == nil {
				p.Alert = "Room not found."
				elem.Dirty(p)
				return nil
			}
			if err := room.Judge(p.App.playerID(p.Session), p.SelectedSubmission); err != nil {
				p.Alert = err.Error()
				elem.Dirty(p)
				return nil
			}
			p.SelectedSubmission = ""
			p.Alert = ""
			p.App.DirtyRoom(room)
			elem.Dirty(p)
			return nil
		})
}

func slicesContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func deleteString(values []string, target string) []string {
	out := values[:0]
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

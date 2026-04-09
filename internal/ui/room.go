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

func (p *RoomPage) RoomTag() any { return p.Room() }

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
			p.App.Dirty(p)
			return nil
		}
		p.App.setNickname(p.Session, name)
		if room, err := p.App.joinRoom(p.Session, p.RoomCode); err == nil {
			elem.Request.Redirect(p.App.roomURL(room.Code()))
			return nil
		} else {
			p.Alert = err.Error()
		}
		p.App.Dirty(p, p.App, p.RoomTag())
		return nil
	})
}

func (p *RoomPage) StartGameAction() bind.Binder[string] {
	label := "Start Game"
	return bind.New(&p.mu, &label).
		GetLocked(func(bind bind.Binder[string], elem *jaws.Element) string {
			snap := p.Snapshot()
			if !snap.CanStart {
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
				p.App.Dirty(p)
				return nil
			}
			if err := room.Start(p.App.playerID(p.Session)); err != nil {
				p.Alert = err.Error()
				p.App.Dirty(p)
				return nil
			}
			p.SelectedCardIDs = nil
			p.SelectedSubmission = ""
			p.Alert = ""
			p.App.Dirty(p, p.App, room)
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
				p.App.Dirty(p)
				return nil
			}
			if err := room.ToggleDeck(p.App.playerID(p.Session), deckID, value); err != nil {
				p.Alert = err.Error()
				p.App.Dirty(p)
				return nil
			}
			p.Alert = ""
			p.App.Dirty(p, p.App, room)
			return nil
		})
}

func (p *RoomPage) CardAction(card deck.WhiteCard) bind.Binder[string] {
	label := card.Text
	return bind.New(&p.mu, &label).
		GetLocked(func(bind bind.Binder[string], elem *jaws.Element) string {
			snap := p.Snapshot()
			if !snap.CanSubmit {
				elem.SetAttr("disabled", "")
			} else {
				elem.RemoveAttr("disabled")
			}
			if slicesContains(p.SelectedCardIDs, card.ID) {
				elem.SetClass("is-selected")
			} else {
				elem.RemoveClass("is-selected")
			}
			return label
		}).
		Clicked(func(bind bind.Binder[string], elem *jaws.Element, _ string) error {
			snap := p.Snapshot()
			if !snap.CanSubmit {
				return nil
			}
			if slicesContains(p.SelectedCardIDs, card.ID) {
				p.SelectedCardIDs = deleteString(p.SelectedCardIDs, card.ID)
			} else {
				if len(p.SelectedCardIDs) >= snap.NeedPick {
					p.Alert = fmt.Sprintf("Select exactly %d cards.", snap.NeedPick)
					p.App.Dirty(p)
					return nil
				}
				p.SelectedCardIDs = append(p.SelectedCardIDs, card.ID)
				p.Alert = ""
			}
			p.App.Dirty(p)
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
				p.App.Dirty(p)
				return nil
			}
			if err := room.PlayCards(p.App.playerID(p.Session), append([]string(nil), p.SelectedCardIDs...)); err != nil {
				p.Alert = err.Error()
				p.App.Dirty(p)
				return nil
			}
			p.SelectedCardIDs = nil
			p.Alert = ""
			p.App.Dirty(p, p.App, room)
			return nil
		})
}

func (p *RoomPage) SubmissionAction(sub game.SubmissionView) bind.Binder[string] {
	label := joinSubmission(sub.Cards)
	return bind.New(&p.mu, &label).
		GetLocked(func(bind bind.Binder[string], elem *jaws.Element) string {
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
			return label
		}).
		Clicked(func(bind bind.Binder[string], elem *jaws.Element, _ string) error {
			if !p.Snapshot().CanJudge {
				return nil
			}
			if p.SelectedSubmission == sub.ID {
				p.SelectedSubmission = ""
			} else {
				p.SelectedSubmission = sub.ID
			}
			p.App.Dirty(p)
			return nil
		})
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
				p.App.Dirty(p)
				return nil
			}
			if err := room.Judge(p.App.playerID(p.Session), p.SelectedSubmission); err != nil {
				p.Alert = err.Error()
				p.App.Dirty(p)
				return nil
			}
			p.SelectedSubmission = ""
			p.Alert = ""
			p.App.Dirty(p, p.App, room)
			return nil
		})
}

func joinSubmission(cards []deck.WhiteCard) string {
	parts := make([]string, 0, len(cards))
	for _, card := range cards {
		parts = append(parts, card.Text)
	}
	return strings.Join(parts, " / ")
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

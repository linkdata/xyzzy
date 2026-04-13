package ui

import (
	"errors"
	"fmt"
	"html/template"
	"sync"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/jaws/lib/jtag"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type taggedBinder[T comparable] struct {
	bind.Binder[T]
	tag any
}

func (b taggedBinder[T]) JawsGetTag(jtag.Context) any {
	return b.tag
}

type roomDeckTag struct {
	Room *game.Room
	Deck *deck.Deck
}

func (a *App) roomMutation(player *game.Player, elem *jaws.Element, mutate func(*game.Room) error, after func(*game.Room)) error {
	room := player.Room
	if room == nil {
		return game.ErrRoomNotFound
	}
	if err := mutate(room); err != nil {
		return err
	}
	if after != nil {
		after(room)
	}
	elem.Dirty(player, room)
	return nil
}

func (a *App) DeckToggle(player *game.Player, deck *deck.Deck) bind.Binder[bool] {
	value := false
	room := player.Room
	binder := bind.New(&sync.Mutex{}, &value).
		GetLocked(func(bind bind.Binder[bool], elem *jaws.Element) bool {
			return room != nil && room.DeckEnabled(deck)
		}).
		SetLocked(func(bind bind.Binder[bool], elem *jaws.Element, value bool) error {
			return a.roomMutation(player, elem, func(room *game.Room) error {
				return room.SetDeckEnabled(player, deck, value)
			}, nil)
		})
	return taggedBinder[bool]{Binder: binder, tag: roomDeckTag{Room: room, Deck: deck}}
}

func (a *App) CardAction(player *game.Player, card *deck.WhiteCard) jaws.ClickHandler {
	return jui.Clickable("Select Card", func(elem *jaws.Element, name string) error {
		room := player.Room
		if room == nil || !room.CanSubmit(player) || card == nil {
			return nil
		}
		changed, alert := applyCardSelection(player, card.ID, room.NeedPick())
		if alert != "" {
			return errors.New(alert)
		}
		if changed {
			elem.Dirty(player)
		}
		return nil
	})
}

func (a *App) SubmissionAction(player *game.Player, submission *game.Submission) jaws.ClickHandler {
	return jui.Clickable("Select Submission", func(elem *jaws.Element, name string) error {
		room := player.Room
		if room == nil || !room.CanJudge(player) || submission == nil {
			return nil
		}
		if player.SelectedSubmission == submission {
			player.SelectedSubmission = nil
		} else {
			player.SelectedSubmission = submission
		}
		elem.Dirty(player)
		return nil
	})
}

func (a *App) DeckToggleAttrs(player *game.Player) template.HTMLAttr {
	room := player.Room
	if room == nil || !room.IsHost(player) || room.State() != game.StateLobby {
		return `disabled`
	}
	return ""
}

func (a *App) HandCardAttrs(player *game.Player) template.HTMLAttr {
	room := player.Room
	if room == nil || !room.CanSubmit(player) {
		return `disabled`
	}
	return ""
}

func (a *App) HandCardClass(player *game.Player, card *deck.WhiteCard) template.HTMLAttr {
	class := `class="card-face card-face-white w-100 text-start`
	if card != nil && slicesContains(player.SelectedCardIDs, card.ID) {
		class += ` is-selected`
	}
	return template.HTMLAttr(class + `"`)
}

func (a *App) SubmissionAttrs(player *game.Player) template.HTMLAttr {
	room := player.Room
	if room == nil || !room.CanJudge(player) {
		return `disabled`
	}
	return ""
}

func (a *App) SubmissionClass(player *game.Player, submission *game.Submission) template.HTMLAttr {
	class := `class="card-face card-face-white w-100 text-start`
	if room := player.Room; room != nil && room.IsWinningSubmission(submission) {
		class += ` is-winning`
	}
	if player.SelectedSubmission == submission {
		class += ` is-selected`
	}
	return template.HTMLAttr(class + `"`)
}

func applyCardSelection(player *game.Player, cardID string, needPick int) (changed bool, alert string) {
	if slicesContains(player.SelectedCardIDs, cardID) {
		player.SelectedCardIDs = deleteString(player.SelectedCardIDs, cardID)
		return true, ""
	}
	if needPick == 1 {
		player.SelectedCardIDs = []string{cardID}
		return true, ""
	}
	if len(player.SelectedCardIDs) >= needPick {
		return false, fmt.Sprintf("Select exactly %d cards.", needPick)
	}
	player.SelectedCardIDs = append(player.SelectedCardIDs, cardID)
	return true, ""
}

func waitingTitle(player *game.Player, room *game.Room) string {
	if room == nil {
		return "Waiting"
	}
	switch room.State() {
	case game.StateJudging:
		if judge := room.JudgeName(); judge != "" {
			return judge + " is picking the winner"
		}
		return "Waiting for the judge"
	case game.StatePlaying:
		if room.IsJudge(player) {
			return "Waiting for answers"
		}
		return "Waiting for the rest of the table"
	default:
		return "Waiting"
	}
}

func waitingDetail(player *game.Player, room *game.Room) string {
	if room == nil || room.State() != game.StatePlaying {
		return ""
	}
	if room.IsJudge(player) {
		return "You'll choose the winner once every answer is in."
	}
	if room.SubmittedBy(player) {
		return "Your cards are in."
	}
	return ""
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

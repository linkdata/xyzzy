package ui

import (
	"github.com/linkdata/jaws"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type templateDot struct {
	App    *App
	Player *game.Player
}

// JawsGetTag leaves live-region dependencies to the rendered children.
func (templateDot) JawsGetTag() any { return nil }

type roomTemplateDot struct {
	templateDot
	Room *game.Room
}

func (d roomTemplateDot) JawsGetTag() (result any) {
	if d.Room != nil {
		result = d.Room
	}
	return
}

type gameTemplateDot struct {
	roomTemplateDot
}

func (d templateDot) OnlineCount() (result int) {
	result = d.App.Jaws.SessionCount()
	return
}

func (d gameTemplateDot) JawsGetTag() any {
	return []any{d.Player, d.Room}
}

func (d templateDot) RoomSidebar(code string) (result jaws.Container) {
	result = roomSection{
		App:           d.App,
		Player:        d.Player,
		RequestedCode: normalizeRoomCode(code),
		Kind:          roomSectionSidebar,
	}
	return
}

func (d templateDot) RoomMain(code string) (result jaws.Container) {
	result = roomSection{
		App:           d.App,
		Player:        d.Player,
		RequestedCode: normalizeRoomCode(code),
		Kind:          roomSectionMain,
	}
	return
}

func (d templateDot) SaveNicknameButton() (result jui.Object) {
	result = jui.New("Save Nickname").Clicked(func(_ jui.Object, elem *jaws.Element, _ jaws.Click) (err error) {
		d.App.setNickname(d.Player, d.Player.NicknameInputValue())
		d.App.Jaws.Dirty(d.App.Manager, d.Player, d.Player.Room())
		redirectURL := elem.Request.Initial().URL.RequestURI()
		if redirectURL == "" {
			redirectURL = "/"
		}
		elem.Request.Redirect(redirectURL)
		return
	})
	return
}

func (d templateDot) CreateRoomButton() (result jui.Object) {
	result = jui.New("Create Room").Clicked(func(_ jui.Object, elem *jaws.Element, _ jaws.Click) (err error) {
		if current := d.Player.Room(); current != nil {
			elem.Request.Redirect(d.App.RoomURL(current.Code()))
			return
		}
		if _, ok := d.App.createRoomLimiter.Allow(clientIP(elem.Request.Initial())); !ok {
			elem.Request.Alert("warning", "Please wait before creating another room.")
			return
		}
		var room *game.Room
		if room, err = d.App.createRoom(d.Player); err == nil {
			elem.Request.Redirect(d.App.RoomURL(room.Code()))
		}
		return
	})
	return
}

func (d templateDot) DisplayNickname() (result string) {
	if d.Player != nil {
		if room := d.Player.Room(); room != nil {
			result = room.NicknameFor(d.Player)
			if result != "" {
				return
			}
		}
		result = d.Player.NicknameValue()
	}
	return
}

func (d gameTemplateDot) DeckInput(selectedDeck *deck.Deck) (result deckInput) {
	result = deckInput{Room: d.Room, Player: d.Player, Deck: selectedDeck}
	return
}

func (d gameTemplateDot) HandCardViews() (result []whiteCardView) {
	cards := d.Room.HandFor(d.Player)
	enabled := d.Room.CanSubmit(d.Player)
	result = make([]whiteCardView, 0, len(cards))
	for _, card := range cards {
		result = append(result, whiteCardView{
			Room:           d.Room,
			Player:         d.Player,
			Card:           card,
			SelectionOrder: d.Room.SelectionOrderFor(d.Player, card),
			Enabled:        enabled,
		})
	}
	return
}

func (d gameTemplateDot) SubmissionViews() (result []submissionView) {
	submissions := d.Room.Submissions()
	result = make([]submissionView, 0, len(submissions))
	for _, submission := range submissions {
		result = append(result, submissionView{Room: d.Room, Player: d.Player, Submission: submission})
	}
	return
}

func (d gameTemplateDot) WaitingTitle() (result string) {
	switch d.Room.State() {
	case game.StateJudging:
		if judge := d.Room.JudgeName(); judge != "" {
			result = judge + " is picking the winner"
			return
		}
		result = "Waiting for the judge"
	case game.StatePlaying:
		if d.Room.IsJudge(d.Player) {
			result = "Waiting for answers"
			return
		}
		result = "Waiting for the rest of the table"
	default:
		result = "Waiting"
	}
	return
}

func (d gameTemplateDot) WaitingDetail() (result string) {
	if d.Room.State() != game.StatePlaying {
		return
	}
	if d.Room.IsJudge(d.Player) {
		result = "You'll choose the winner once every answer is in."
		return
	}
	if d.Room.SubmittedBy(d.Player) {
		result = "Your cards are in."
	}
	return
}

func (d gameTemplateDot) BlackFootnote(card *deck.BlackCard) (result string) {
	result = cardFootnote(d.Room.FirstSelectedDeckNameForBlackCard(card), card.ID)
	return
}

func (d roomTemplateDot) StateBadgeClass() (result string) {
	switch d.Room.State() {
	case game.StateLobby:
		result = "bg-secondary"
	case game.StatePlaying:
		result = "bg-success"
	case game.StateReview:
		result = "bg-info text-dark"
	default:
		result = "bg-warning text-dark"
	}
	return
}

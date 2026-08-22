package ui

import (
	"errors"
	"strconv"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type templateDot struct {
	App    *App
	Player *game.Player
}

type roomPageDot struct {
	templateDot
	RequestedCode string
	RequestedRoom *game.Room
}

var _ jaws.ConnectHandler = roomPageDot{}

func (d roomPageDot) JawsConnect(rq *jaws.Request) (err error) {
	if current := d.Player.Room(); current != nil {
		if current != d.RequestedRoom {
			rq.Redirect(d.App.RoomURL(current.Code()))
		}
		return
	}
	if d.App.Manager.Room(d.RequestedCode) != d.RequestedRoom {
		rq.Redirect(d.App.RoomURL(d.RequestedCode))
		return
	}
	if d.RequestedRoom == nil {
		return
	}
	var joined *game.Room
	if joined, err = d.App.joinRoom(d.Player, d.RequestedCode); err == nil {
		if joined != d.RequestedRoom {
			rq.Redirect(d.App.RoomURL(joined.Code()))
		}
		return
	}
	if current := d.Player.Room(); current != nil {
		err = nil
		if current != d.RequestedRoom {
			rq.Redirect(d.App.RoomURL(current.Code()))
		}
		return
	}
	if d.App.Manager.Room(d.RequestedCode) != d.RequestedRoom {
		err = nil
		rq.Redirect(d.App.RoomURL(d.RequestedCode))
		return
	}
	// These failures are normal rendered observer states. Returning one would
	// close the WebSocket. Any intervening state change has already published its
	// dependencies to pending and active Requests.
	switch {
	case errors.Is(err, game.ErrAlreadyInRoom),
		errors.Is(err, game.ErrRoomNotFound),
		errors.Is(err, game.ErrRoomFull),
		errors.Is(err, game.ErrNotEnoughWhiteCards):
		err = nil
	}
	return
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
	templateDot
	Room *game.Room
}

func (d gameTemplateDot) JawsGetTag() any {
	return []any{d.Player, d.Room}
}

func (d roomPageDot) RoomSidebar() (result jaws.Container) {
	result = roomSection{
		App:           d.App,
		Player:        d.Player,
		RequestedCode: d.RequestedCode,
		RequestedRoom: d.RequestedRoom,
		Kind:          roomSectionSidebar,
	}
	return
}

func (d roomPageDot) RoomMain() (result jaws.Container) {
	result = roomSection{
		App:           d.App,
		Player:        d.Player,
		RequestedCode: d.RequestedCode,
		RequestedRoom: d.RequestedRoom,
		Kind:          roomSectionMain,
	}
	return
}

func (d templateDot) SaveNicknameButton() (result jui.Object) {
	result = jui.New("Save Nickname").Clicked(func(_ jui.Object, elem *jaws.Element, _ jaws.Click) (err error) {
		d.App.Manager.SetNickname(d.Player, d.Player.NicknameInputValue())
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

func (d templateDot) DisplayNickname() (result bind.Getter[string]) {
	result = bind.StringGetterFunc(func(*jaws.Element) (nickname string) {
		nickname = d.App.playerNickname(d.Player)
		return
	}, d.Player)
	return
}

func (d templateDot) OnlineCount() (result bind.Getter[string]) {
	result = bind.StringGetterFunc(func(*jaws.Element) (count string) {
		count = strconv.Itoa(d.App.Jaws.ActiveSessionCount())
		return
	}, d.App.Jaws.ActiveSessionCountTag())
	return
}

func (d gameTemplateDot) DeckInput(selectedDeck *deck.Deck) (result deckInput) {
	result = deckInput{Room: d.Room, Player: d.Player, Deck: selectedDeck}
	return
}

func (d gameTemplateDot) HandCardViews() (result []whiteCardView) {
	cards := d.Room.HandFor(d.Player)
	result = make([]whiteCardView, 0, len(cards))
	for _, card := range cards {
		result = append(result, whiteCardView{
			Room:   d.Room,
			Player: d.Player,
			Card:   card,
		})
	}
	return
}

func (d gameTemplateDot) SubmissionViews() (result []submissionView) {
	submissions := d.Room.Submissions()
	result = make([]submissionView, 0, len(submissions))
	for _, submission := range submissions {
		result = append(result, submissionView{
			Room:       d.Room,
			Player:     d.Player,
			Submission: submission,
		})
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
		if d.Room.SubmittedBy(d.Player) {
			result = "Waiting for the rest of the table"
			return
		}
		result = "Waiting"
	default:
		result = "Waiting"
	}
	return
}

func (d gameTemplateDot) WaitingDetail() (result string) {
	if d.Room.State() == game.StatePlaying {
		if d.Room.IsJudge(d.Player) {
			result = "You'll choose the winner once every answer is in."
			return
		}
		if d.Room.SubmittedBy(d.Player) {
			result = "Your cards are in."
		}
	}
	return
}

func (d gameTemplateDot) BlackFootnote(card *deck.BlackCard) (result string) {
	result = cardFootnote(d.Room.FirstSelectedDeckNameForBlackCard(card), card.ID)
	return
}

func (roomTemplateDot) StateBadgeClass(state game.RoomState) (result string) {
	switch state {
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

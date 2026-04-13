package ui

import (
	"errors"
	"html/template"
	"strings"
	"sync"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type templateDot struct {
	App *App
	*game.Player
	*game.Room
}

func (d templateDot) OnlinePlayers() int {
	return d.App.Jaws.SessionCount()
}

func (d templateDot) PublicRooms() []*game.Room {
	return d.App.Manager.PublicRooms()
}

func (d templateDot) RoomByCode(code string) *game.Room {
	return d.App.Manager.Room(code)
}

func (d templateDot) LobbySidebar() jaws.Container {
	return &section{App: d.App, Player: d.Player, Kind: sectionLobbySidebar}
}

func (d templateDot) LobbyMain() jaws.Container {
	return &section{App: d.App, Player: d.Player, Kind: sectionLobbyMain}
}

func (d templateDot) RoomSidebar(code string) jaws.Container {
	return &section{
		App:           d.App,
		Player:        d.Player,
		RequestedCode: normalizeRoomCode(code),
		Kind:          sectionRoomSidebar,
	}
}

func (d templateDot) RoomMain(code string) jaws.Container {
	return &section{
		App:           d.App,
		Player:        d.Player,
		RequestedCode: normalizeRoomCode(code),
		Kind:          sectionRoomMain,
	}
}

func (d templateDot) SaveNicknameClick() jaws.ClickHandler {
	return jui.Clickable("Save Nickname", func(elem *jaws.Element, name string) error {
		d.App.setNickname(d.Player, d.Player.NicknameInput)
		d.App.Jaws.Dirty(d.App.Manager, d.Player, d.Player.Room)
		redirectURL := elem.Request.Initial().URL.RequestURI()
		if redirectURL == "" {
			redirectURL = "/"
		}
		elem.Request.Redirect(redirectURL)
		return nil
	})
}

func (d templateDot) OrderedDecks() []*deck.Deck {
	return d.App.Catalog.OrderedDecks()
}

func (d templateDot) DeckToggle(deck *deck.Deck) bind.Binder[bool] {
	value := false
	room := d.Room
	binder := bind.New(&sync.Mutex{}, &value).
		GetLocked(func(bind bind.Binder[bool], elem *jaws.Element) bool {
			return room.DeckEnabled(deck)
		}).
		SetLocked(func(bind bind.Binder[bool], elem *jaws.Element, value bool) error {
			if err := room.SetDeckEnabled(d.Player, deck, value); err != nil {
				return err
			}
			elem.Dirty(d.Player, room)
			return nil
		})
	return taggedBinder[bool]{Binder: binder, tag: roomDeckTag{Room: room, Deck: deck}}
}

func (d templateDot) DeckToggleAttrs() template.HTMLAttr {
	if !d.Room.IsHost(d.Player) || d.Room.State() != game.StateLobby {
		return `disabled`
	}
	return ""
}

func (d templateDot) CardAction(card *deck.WhiteCard) jaws.ClickHandler {
	return jui.Clickable("Select Card", func(elem *jaws.Element, name string) error {
		if !d.Room.CanSubmit(d.Player) {
			return nil
		}
		changed, alert := applyCardSelection(d.Player, card, d.Room.NeedPick())
		if alert != "" {
			return errors.New(alert)
		}
		if changed {
			elem.Dirty(d.Player)
		}
		return nil
	})
}

func (d templateDot) CardBody(card *deck.WhiteCard) bind.HTMLGetter {
	return d.App.HandCardHTML(d.Player, card)
}

func (d templateDot) CardAttrs() template.HTMLAttr {
	if !d.Room.CanSubmit(d.Player) {
		return `disabled`
	}
	return ""
}

func (d templateDot) CardClass(card *deck.WhiteCard) template.HTMLAttr {
	class := `class="card-face card-face-white w-100 text-start`
	if slicesContains(d.Player.SelectedCards, card) {
		class += ` is-selected`
	}
	return template.HTMLAttr(class + `"`)
}

func (d templateDot) SubmissionAction(submission *game.Submission) jaws.ClickHandler {
	return jui.Clickable("Select Submission", func(elem *jaws.Element, name string) error {
		if !d.Room.CanJudge(d.Player) {
			return nil
		}
		if d.Player.SelectedSubmission == submission {
			d.Player.SelectedSubmission = nil
		} else {
			d.Player.SelectedSubmission = submission
		}
		elem.Dirty(d.Player)
		return nil
	})
}

func (d templateDot) SubmissionBody(submission *game.Submission) bind.HTMLGetter {
	return d.App.SubmissionHTML(d.Player, submission)
}

func (d templateDot) SubmissionAttrs() template.HTMLAttr {
	if !d.Room.CanJudge(d.Player) {
		return `disabled`
	}
	return ""
}

func (d templateDot) SubmissionClass(submission *game.Submission) template.HTMLAttr {
	class := `class="card-face card-face-white w-100 text-start`
	if d.Room.IsWinningSubmission(submission) {
		class += ` is-winning`
	}
	if d.Player.SelectedSubmission == submission {
		class += ` is-selected`
	}
	return template.HTMLAttr(class + `"`)
}

func (d templateDot) PrivateToggle() bind.Binder[bool] {
	return d.Room.PrivateToggle(d.Player)
}

func (d templateDot) PrivateToggleAttrs() template.HTMLAttr {
	return d.Room.PrivateToggleAttrs(d.Player)
}

func (d templateDot) ScoreTargetSlider() bind.Binder[int] {
	return d.Room.ScoreTargetSlider(d.Player)
}

func (d templateDot) ScoreTargetAttrs() template.HTMLAttr {
	return d.Room.ScoreTargetAttrs(d.Player)
}

func (d templateDot) StartGameClick() jaws.ClickHandler {
	return d.Room.StartGameClick(d.Player)
}

func (d templateDot) StartGameAttrs() template.HTMLAttr {
	return d.Room.StartGameAttrs(d.Player)
}

func (d templateDot) CanSubmit() bool {
	return d.Room.CanSubmit(d.Player)
}

func (d templateDot) SubmitCardsClick() jaws.ClickHandler {
	return d.Room.SubmitCardsClick(d.Player)
}

func (d templateDot) SubmitCardsAttrs() template.HTMLAttr {
	return d.Room.SubmitCardsAttrs(d.Player)
}

func (d templateDot) HandFor() []*deck.WhiteCard {
	return d.Room.HandFor(d.Player)
}

func (d templateDot) CanJudge() bool {
	return d.Room.CanJudge(d.Player)
}

func (d templateDot) JudgeClick() jaws.ClickHandler {
	return d.Room.JudgeClick(d.Player)
}

func (d templateDot) JudgeAttrs() template.HTMLAttr {
	return d.Room.JudgeAttrs(d.Player)
}

func (d templateDot) CanProceed() bool {
	return d.Room.CanProceed(d.Player)
}

func (d templateDot) ProceedReviewClick() jaws.ClickHandler {
	return d.Room.ProceedReviewClick(d.Player)
}

func (d templateDot) ProceedReviewAttrs() template.HTMLAttr {
	return d.Room.ProceedReviewAttrs(d.Player)
}

func (d templateDot) WaitingTitle() string {
	switch d.Room.State() {
	case game.StateJudging:
		if judge := d.Room.JudgeName(); judge != "" {
			return judge + " is picking the winner"
		}
		return "Waiting for the judge"
	case game.StatePlaying:
		if d.Room.IsJudge(d.Player) {
			return "Waiting for answers"
		}
		return "Waiting for the rest of the table"
	default:
		return "Waiting"
	}
}

func (d templateDot) WaitingDetail() string {
	if d.Room.State() != game.StatePlaying {
		return ""
	}
	if d.Room.IsJudge(d.Player) {
		return "You'll choose the winner once every answer is in."
	}
	if d.Room.SubmittedBy(d.Player) {
		return "Your cards are in."
	}
	return ""
}

func (d templateDot) BlackFootnote(card *deck.BlackCard) string {
	deckName := d.Room.FirstSelectedDeckNameForBlackCard(card)
	number := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, card.ID)
	switch {
	case deckName == "":
		return number
	case number == "":
		return deckName
	default:
		return deckName + " · " + number
	}
}

func (d templateDot) StateBadgeClass() string {
	switch d.Room.State() {
	case game.StateLobby:
		return "bg-secondary"
	case game.StatePlaying:
		return "bg-success"
	case game.StateReview:
		return "bg-info text-dark"
	default:
		return "bg-warning text-dark"
	}
}

func (d templateDot) PlayerHost(player *game.Player) bool {
	return d.Room.IsHost(player)
}

func (d templateDot) PlayerJudge(player *game.Player) bool {
	return d.Room.IsJudge(player)
}

func (d templateDot) PlayerScore(player *game.Player) int {
	return d.Room.ScoreFor(player)
}

func (d templateDot) PlayerSubmitted(player *game.Player) bool {
	return d.Room.SubmittedBy(player)
}

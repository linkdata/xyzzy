package ui

import (
	"html/template"
	"slices"
	"strings"
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

func (b taggedBinder[T]) JawsGetTag(jtag.Context) (result any) {
	result = b.tag
	return
}

type roomDeckTag struct {
	Room *game.Room
	Deck *deck.Deck
}

type templateDot struct {
	App *App
	*game.Player
	*game.Room
}

func (d templateDot) OnlinePlayers() (result int) {
	result = d.App.Jaws.SessionCount()
	return
}

func (d templateDot) PublicRooms() (result []*game.Room) {
	result = d.App.Manager.PublicRooms()
	return
}

func (d templateDot) RoomByCode(code string) (result *game.Room) {
	result = d.App.Manager.Room(code)
	return
}

func (d templateDot) LobbySidebar() (result jaws.Container) {
	result = &section{App: d.App, Player: d.Player, Kind: sectionLobbySidebar}
	return
}

func (d templateDot) LobbyMain() (result jaws.Container) {
	result = &section{App: d.App, Player: d.Player, Kind: sectionLobbyMain}
	return
}

func (d templateDot) RoomSidebar(code string) (result jaws.Container) {
	result = &section{
		App:           d.App,
		Player:        d.Player,
		RequestedCode: normalizeRoomCode(code),
		Kind:          sectionRoomSidebar,
	}
	return
}

func (d templateDot) RoomMain(code string) (result jaws.Container) {
	result = &section{
		App:           d.App,
		Player:        d.Player,
		RequestedCode: normalizeRoomCode(code),
		Kind:          sectionRoomMain,
	}
	return
}

func (d templateDot) SaveNicknameClick() jaws.ClickHandler {
	return jui.Clickable("Save Nickname", func(elem *jaws.Element, name string) (err error) {
		d.App.setNickname(d.Player, d.Player.NicknameInput)
		d.App.Jaws.Dirty(d.App.Manager, d.Player, d.Player.Room)
		redirectURL := elem.Request.Initial().URL.RequestURI()
		if redirectURL == "" {
			redirectURL = "/"
		}
		elem.Request.Redirect(redirectURL)
		return
	})
}

func (d templateDot) OrderedDecks() (result []*deck.Deck) {
	result = d.App.Catalog.OrderedDecks()
	return
}

func (d templateDot) DeckToggle(deck *deck.Deck) (result bind.Binder[bool]) {
	value := false
	room := d.Room
	binder := bind.New(&sync.Mutex{}, &value).
		GetLocked(func(bind bind.Binder[bool], elem *jaws.Element) (result bool) {
			result = room.DeckEnabled(deck)
			return
		}).
		SetLocked(func(bind bind.Binder[bool], elem *jaws.Element, value bool) (err error) {
			if err = room.SetDeckEnabled(d.Player, deck, value); err == nil {
				elem.Dirty(d.Player, room)
			}
			return
		})
	result = taggedBinder[bool]{Binder: binder, tag: roomDeckTag{Room: room, Deck: deck}}
	return
}

func (d templateDot) DeckToggleAttrs() (result template.HTMLAttr) {
	if !d.Room.IsHost(d.Player) || d.Room.State() != game.StateLobby {
		result = `disabled`
	}
	return
}

func (d templateDot) HandCardView(card *deck.WhiteCard) (result whiteCardView) {
	result = whiteCardView{Room: d.Room, Player: d.Player, Card: card, SelectionOrder: selectionOrder(d.Player, card)}
	return
}

func (d templateDot) HandCardViews() (result []whiteCardView) {
	cards := d.Room.HandFor(d.Player)
	result = make([]whiteCardView, 0, len(cards))
	for _, card := range cards {
		result = append(result, d.HandCardView(card))
	}
	return
}

func (d templateDot) CardAttrs() (result template.HTMLAttr) {
	if !d.Room.CanSubmit(d.Player) {
		result = `disabled`
	}
	return
}

func (d templateDot) CardClass(card *deck.WhiteCard) (result template.HTMLAttr) {
	class := `class="card-face card-face-white w-100 text-start`
	if slices.Contains(d.Player.SelectedCards, card) {
		class += ` is-selected`
	}
	result = template.HTMLAttr(class + `"`)
	return
}

func (d templateDot) SubmissionView(submission *game.Submission) (result submissionView) {
	result = submissionView{Room: d.Room, Player: d.Player, Submission: submission}
	return
}

func (d templateDot) SubmissionViews() (result []submissionView) {
	submissions := d.Room.Submissions()
	result = make([]submissionView, 0, len(submissions))
	for _, submission := range submissions {
		result = append(result, d.SubmissionView(submission))
	}
	return
}

func (d templateDot) SubmissionAttrs() (result template.HTMLAttr) {
	if !d.Room.CanJudge(d.Player) {
		result = `disabled`
	}
	return
}

func (d templateDot) SubmissionClass(submission *game.Submission) (result template.HTMLAttr) {
	class := `class="card-face card-face-white w-100 text-start`
	if d.Room.IsWinningSubmission(submission) {
		class += ` is-winning`
	}
	if d.Player.SelectedSubmission == submission {
		class += ` is-selected`
	}
	result = template.HTMLAttr(class + `"`)
	return
}

func (d templateDot) PrivateToggle() (result bind.Binder[bool]) {
	result = d.Room.PrivateToggle(d.Player)
	return
}

func (d templateDot) PrivateToggleAttrs() (result template.HTMLAttr) {
	result = d.Room.PrivateToggleAttrs(d.Player)
	return
}

func (d templateDot) ScoreTargetSlider() (result bind.Binder[int]) {
	result = d.Room.ScoreTargetSlider(d.Player)
	return
}

func (d templateDot) ScoreTargetAttrs() (result template.HTMLAttr) {
	result = d.Room.ScoreTargetAttrs(d.Player)
	return
}

func (d templateDot) StartGameClick() (result jaws.ClickHandler) {
	result = d.Room.StartGameClick(d.Player)
	return
}

func (d templateDot) StartGameAttrs() (result template.HTMLAttr) {
	result = d.Room.StartGameAttrs(d.Player)
	return
}

func (d templateDot) CanSubmit() (result bool) {
	result = d.Room.CanSubmit(d.Player)
	return
}

func (d templateDot) SubmitCardsClick() (result jaws.ClickHandler) {
	result = d.Room.SubmitCardsClick(d.Player)
	return
}

func (d templateDot) SubmitCardsAttrs() (result template.HTMLAttr) {
	result = d.Room.SubmitCardsAttrs(d.Player)
	return
}

func (d templateDot) HandFor() (result []*deck.WhiteCard) {
	result = d.Room.HandFor(d.Player)
	return
}

func (d templateDot) CanJudge() (result bool) {
	result = d.Room.CanJudge(d.Player)
	return
}

func (d templateDot) JudgeClick() (result jaws.ClickHandler) {
	result = d.Room.JudgeClick(d.Player)
	return
}

func (d templateDot) JudgeAttrs() (result template.HTMLAttr) {
	result = d.Room.JudgeAttrs(d.Player)
	return
}

func (d templateDot) CanProceed() (result bool) {
	result = d.Room.CanProceed(d.Player)
	return
}

func (d templateDot) ProceedReviewClick() (result jaws.ClickHandler) {
	result = d.Room.ProceedReviewClick(d.Player)
	return
}

func (d templateDot) ProceedReviewAttrs() (result template.HTMLAttr) {
	result = d.Room.ProceedReviewAttrs(d.Player)
	return
}

func (d templateDot) WaitingTitle() (result string) {
	switch d.Room.State() {
	case game.StateJudging:
		if judge := d.Room.JudgeName(); judge != "" {
			result = judge + " is picking the winner"
			return
		}
		result = "Waiting for the judge"
		return
	case game.StatePlaying:
		if d.Room.IsJudge(d.Player) {
			result = "Waiting for answers"
			return
		}
		result = "Waiting for the rest of the table"
		return
	default:
		result = "Waiting"
		return
	}
}

func (d templateDot) WaitingDetail() (result string) {
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

func (d templateDot) BlackFootnote(card *deck.BlackCard) (result string) {
	deckName := d.Room.FirstSelectedDeckNameForBlackCard(card)
	number := strings.Map(func(r rune) (result rune) {
		if r >= '0' && r <= '9' {
			result = r
			return
		}
		result = -1
		return

	}, card.ID)
	switch {
	case deckName == "":
		result = number
		return
	case number == "":
		result = deckName
		return
	default:
		result = deckName + " · " + number
		return
	}
}

func (d templateDot) StateBadgeClass() (result string) {
	switch d.Room.State() {
	case game.StateLobby:
		result = "bg-secondary"
		return
	case game.StatePlaying:
		result = "bg-success"
		return
	case game.StateReview:
		result = "bg-info text-dark"
		return
	default:
		result = "bg-warning text-dark"
		return
	}
}

func (d templateDot) PlayerHost(player *game.Player) (result bool) {
	result = d.Room.IsHost(player)
	return
}

func (d templateDot) PlayerJudge(player *game.Player) (result bool) {
	result = d.Room.IsJudge(player)
	return
}

func (d templateDot) PlayerScore(player *game.Player) (result int) {
	result = d.Room.ScoreFor(player)
	return
}

func (d templateDot) PlayerSubmitted(player *game.Player) (result bool) {
	result = d.Room.SubmittedBy(player)
	return
}

func selectionOrder(player *game.Player, card *deck.WhiteCard) (result int) {
	for i, selected := range player.SelectedCards {
		if selected == card {
			result = i + 1
			return
		}
	}
	return
}

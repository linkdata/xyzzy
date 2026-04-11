package ui

import (
	"bytes"
	"html/template"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type whiteCardView struct {
	Room           *game.Room
	Card           *deck.WhiteCard
	SelectionOrder int
}

type submissionCardsView struct {
	Cards []whiteCardView
}

func (a *App) HandCardHTML(player *game.Player, card *deck.WhiteCard) bind.HTMLGetter {
	room := (*game.Room)(nil)
	if player != nil {
		room = player.Room
	}
	tag := HandCardRef{Player: player, Room: room, Card: card}
	dot := whiteCardView{Room: room, Card: card, SelectionOrder: selectionOrder(player, card)}
	return bind.HTMLGetterFunc(func(elem *jaws.Element) template.HTML {
		return renderTemplateHTML(elem, "white_card_body.html", dot)
	}, tag)
}

func (a *App) SubmissionHTML(player *game.Player, submission *game.Submission) bind.HTMLGetter {
	room := (*game.Room)(nil)
	if player != nil {
		room = player.Room
	}
	tag := SubmissionRef{Player: player, Room: room, Submission: submission}
	dot := submissionCardsView{Cards: submissionCardViews(room, submission)}
	return bind.HTMLGetterFunc(func(elem *jaws.Element) template.HTML {
		return renderTemplateHTML(elem, "submission_cards_body.html", dot)
	}, tag)
}

func submissionCardViews(room *game.Room, submission *game.Submission) []whiteCardView {
	if room == nil || submission == nil {
		return nil
	}
	cards := room.SubmissionCards(submission)
	views := make([]whiteCardView, 0, len(cards))
	for _, card := range cards {
		if card != nil {
			views = append(views, whiteCardView{Room: room, Card: card})
		}
	}
	return views
}

func renderTemplateHTML(elem *jaws.Element, name string, dot any) template.HTML {
	if elem == nil || elem.Request == nil || elem.Request.Jaws == nil {
		return ""
	}
	tmpl := elem.Request.Jaws.LookupTemplate(name)
	if tmpl == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, dot); err != nil {
		elem.Request.Jaws.Log(err)
		return ""
	}
	return template.HTML(buf.String()) // #nosec G203
}

func selectionOrder(player *game.Player, card *deck.WhiteCard) int {
	if player == nil || card == nil {
		return 0
	}
	for i, cardID := range player.SelectedCardIDs {
		if cardID == card.ID {
			return i + 1
		}
	}
	return 0
}

package ui

import (
	"bytes"
	"html/template"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type submissionCardsView struct {
	Cards []whiteCardView
}

func (a *App) HandCardHTML(player *game.Player, card *deck.WhiteCard) bind.HTMLGetter {
	room := player.Room
	tag := HandCardRef{Player: player, Room: room, Card: card}
	dot := whiteCardView{Room: room, Player: player, Card: card, SelectionOrder: selectionOrder(player, card)}
	return bind.HTMLGetterFunc(func(elem *jaws.Element) template.HTML {
		return renderTemplateHTML(elem, "white_card_body.html", dot)
	}, tag)
}

func (a *App) SubmissionHTML(player *game.Player, submission *game.Submission) bind.HTMLGetter {
	room := player.Room
	tag := SubmissionRef{Player: player, Room: room, Submission: submission}
	dot := submissionCardsView{Cards: submissionCardViews(room, submission)}
	return bind.HTMLGetterFunc(func(elem *jaws.Element) template.HTML {
		return renderTemplateHTML(elem, "submission_cards_body.html", dot)
	}, tag)
}

func submissionCardViews(room *game.Room, submission *game.Submission) []whiteCardView {
	cards := room.SubmissionCards(submission)
	views := make([]whiteCardView, 0, len(cards))
	for _, card := range cards {
		views = append(views, whiteCardView{Room: room, Card: card})
	}
	return views
}

func renderTemplateHTML(elem *jaws.Element, name string, dot any) template.HTML {
	tmpl := elem.Request.Jaws.LookupTemplate(name)
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, dot); err != nil {
		elem.Request.Jaws.Log(err)
		return ""
	}
	return template.HTML(buf.String()) // #nosec G203
}

func selectionOrder(player *game.Player, card *deck.WhiteCard) int {
	for i, selected := range player.SelectedCards {
		if selected == card {
			return i + 1
		}
	}
	return 0
}

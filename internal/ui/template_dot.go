package ui

import (
	"html/template"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type templateDot struct {
	*game.Player
	*game.Room
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

package ui

import (
	"html/template"

	"github.com/linkdata/jaws/lib/bind"
)

type uiScoreTarget struct {
	bind.Binder[int]
	ishost bool
}

func (ui *uiScoreTarget) HostEnabled() template.HTMLAttr {
	if !ui.ishost {
		return "disabled"
	}
	return ""
}

func (p *RoomPage) ScoreTargetSlider() *uiScoreTarget {
	return &uiScoreTarget{
		ishost: p.Snapshot().IsHost,
		Binder: p.Room().BindTargetScore(),
	}
}

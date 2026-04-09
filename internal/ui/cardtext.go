package ui

import (
	"html"
	"html/template"
	"strings"
)

var legacyCardTagReplacer = strings.NewReplacer(
	"&lt;i&gt;", "<i>",
	"&lt;/i&gt;", "</i>",
	"&lt;em&gt;", "<em>",
	"&lt;/em&gt;", "</em>",
	"&lt;b&gt;", "<b>",
	"&lt;/b&gt;", "</b>",
	"&lt;strong&gt;", "<strong>",
	"&lt;/strong&gt;", "</strong>",
	"&lt;br&gt;", "<br>",
	"&lt;br/&gt;", "<br/>",
	"&lt;br /&gt;", "<br />",
)

func formatCardHTML(text string) template.HTML {
	escaped := html.EscapeString(html.UnescapeString(text))
	return template.HTML(legacyCardTagReplacer.Replace(escaped)) // #nosec G203
}

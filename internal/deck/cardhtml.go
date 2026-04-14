package deck

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

func formatCardHTML(text string) (result template.HTML) {
	escaped := html.EscapeString(html.UnescapeString(text))
	result = template.HTML(legacyCardTagReplacer.Replace(escaped))
	return
	// #nosec G203
}

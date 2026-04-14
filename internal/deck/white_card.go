package deck

import "html/template"

// WhiteCard is a white answer card loaded from embedded JSON.
type WhiteCard struct {
	ID        string        `json:"id"`
	Text      string        `json:"text"`
	HTML      template.HTML `json:"-"`
	Watermark string        `json:"watermark,omitempty"`
}

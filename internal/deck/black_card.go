package deck

import "html/template"

// BlackCard is a black prompt card loaded from embedded JSON.
type BlackCard struct {
	ID        string        `json:"id"`
	Text      string        `json:"text"`
	HTML      template.HTML `json:"-"`
	Pick      int           `json:"pick"`
	Draw      int           `json:"draw"`
	Watermark string        `json:"watermark,omitempty"`
}

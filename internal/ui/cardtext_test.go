package ui

import "testing"

func TestFormatCardHTMLAllowsLegacyFormattingTags(t *testing.T) {
	got := string(formatCardHTML(`Today on <i>Maury</i><br>p&lt;.05`))
	want := `Today on <i>Maury</i><br>p&lt;.05`
	if got != want {
		t.Fatalf("formatCardHTML() = %q, want %q", got, want)
	}
}

func TestFormatCardHTMLEscapesUnexpectedTags(t *testing.T) {
	got := string(formatCardHTML(`<script>alert(1)</script><i>ok</i>`))
	want := `&lt;script&gt;alert(1)&lt;/script&gt;<i>ok</i>`
	if got != want {
		t.Fatalf("formatCardHTML() = %q, want %q", got, want)
	}
}

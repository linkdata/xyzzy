package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSQLAndWriteAssets(t *testing.T) {
	dir := t.TempDir()
	sqlPath := filepath.Join(dir, "cards.sql")
	sql := `COPY black_cards (id, draw, pick, text, watermark) FROM stdin;
1	0	1	Question?	US
\.
COPY white_cards (id, text, watermark) FROM stdin;
1	Answer	US
2	Another	US
\.
COPY card_set (id, active, base_deck, description, name, weight) FROM stdin;
2	t	f	Base Game (US)	Base Game (US)	1
\.
COPY card_set_black_card (card_set_id, black_card_id) FROM stdin;
2	1
\.
COPY card_set_white_card (card_set_id, white_card_id) FROM stdin;
2	1
2	2
\.
`
	if err := os.WriteFile(sqlPath, []byte(sql), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := parseSQL(sqlPath)
	if err != nil {
		t.Fatalf("parseSQL() error = %v", err)
	}
	outDir := filepath.Join(dir, "assets")
	if err := writeAssets(outDir, data); err != nil {
		t.Fatalf("writeAssets() error = %v", err)
	}
	for _, rel := range []string{
		"cards/black/pyx-b-1.json",
		"cards/white/pyx-w-1.json",
		"cards/white/pyx-w-2.json",
		"decks/base-game-us/deck.json",
		"decks/base-game-us/black.json",
		"decks/base-game-us/white.json",
	} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}
}

func TestParseSQLRejectsInvalidIntegers(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "black card id",
			sql: `COPY black_cards (id, draw, pick, text, watermark) FROM stdin;
x	0	1	Question?	US
\.
`,
			want: "invalid black card id",
		},
		{
			name: "white card id",
			sql: `COPY white_cards (id, text, watermark) FROM stdin;
x	Answer	US
\.
`,
			want: "invalid white card id",
		},
		{
			name: "deck weight",
			sql: `COPY card_set (id, active, base_deck, description, name, weight) FROM stdin;
2	t	f	Base Game (US)	Base Game (US)	x
\.
`,
			want: "invalid deck weight",
		},
		{
			name: "deck black link",
			sql: `COPY card_set_black_card (card_set_id, black_card_id) FROM stdin;
2	x
\.
`,
			want: "invalid deck black card id",
		},
		{
			name: "deck white link",
			sql: `COPY card_set_white_card (card_set_id, white_card_id) FROM stdin;
x	1
\.
`,
			want: "invalid deck white deck id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			sqlPath := filepath.Join(dir, "cards.sql")
			if err := os.WriteFile(sqlPath, []byte(tt.sql), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := parseSQL(sqlPath)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parseSQL() error = %v, want %q", err, tt.want)
			}
		})
	}
}

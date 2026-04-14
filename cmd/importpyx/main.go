package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/linkdata/xyzzy/internal/deck"
)

func main() {
	inPath := flag.String("in", "", "path to PretendYoureXyzzy cah_cards.sql")
	outDir := flag.String("out", "assets", "output directory")
	flag.Parse()

	if *inPath == "" {
		fmt.Fprintln(os.Stderr, "-in is required")
		os.Exit(2)
	}

	data, err := parseSQL(*inPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writeAssets(*outDir, data); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseSQL(path string) (data *sqlData, err error) {
	var f *os.File
	if f, err = os.Open(path); err == nil {
		defer f.Close()
		tmp_data := &sqlData{
			blackCards:     make(map[int]deck.BlackCard),
			whiteCards:     make(map[int]deck.WhiteCard),
			decks:          make(map[int]deckRecord),
			deckBlackLinks: make(map[int][]int),
			deckWhiteLinks: make(map[int][]int),
		}
		scanner := bufio.NewScanner(f)
		section := ""
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "COPY black_cards "):
				section = "black"
				continue
			case strings.HasPrefix(line, "COPY white_cards "):
				section = "white"
				continue
			case strings.HasPrefix(line, "COPY card_set ("):
				section = "deck"
				continue
			case strings.HasPrefix(line, "COPY card_set_black_card "):
				section = "deck_black"
				continue
			case strings.HasPrefix(line, "COPY card_set_white_card "):
				section = "deck_white"
				continue
			case line == `\.`:
				section = ""
				continue
			}
			if section == "" || strings.TrimSpace(line) == "" || strings.HasPrefix(line, "--") {
				continue
			}
			parts := strings.Split(line, "\t")
			switch section {
			case "black":
				if len(parts) < 5 {
					err = fmt.Errorf("invalid black card row: %q", line)
					return
				}
				id := mustAtoi(parts[0])
				tmp_data.blackCards[id] = deck.BlackCard{
					ID:        fmt.Sprintf("pyx-b-%d", id),
					Text:      parts[3],
					Pick:      mustAtoi(parts[2]),
					Draw:      mustAtoi(parts[1]),
					Watermark: parts[4],
				}
			case "white":
				if len(parts) < 3 {
					err = fmt.Errorf("invalid white card row: %q", line)
					return
				}
				id := mustAtoi(parts[0])
				tmp_data.whiteCards[id] = deck.WhiteCard{
					ID:        fmt.Sprintf("pyx-w-%d", id),
					Text:      parts[1],
					Watermark: parts[2],
				}
			case "deck":
				if len(parts) < 6 {
					err = fmt.Errorf("invalid deck row: %q", line)
					return
				}
				id := mustAtoi(parts[0])
				name := parts[4]
				deckID := slugify(name)
				if deckID == "" {
					deckID = fmt.Sprintf("deck-%d", id)
				}
				if _, exists := tmp_data.decks[id]; exists {
					err = fmt.Errorf("duplicate deck row id %d", id)
					return
				}
				tmp_data.decks[id] = deckRecord{
					active: parts[1] == "t",
					meta: deck.DeckMetadata{
						ID:               deckID,
						Name:             name,
						Description:      parts[3],
						Weight:           mustAtoi(parts[5]),
						BaseDeck:         parts[2] == "t",
						EnabledByDefault: name == "Base Game (US)",
					},
				}
			case "deck_black":
				if len(parts) < 2 {
					err = fmt.Errorf("invalid deck black row: %q", line)
					return
				}
				deckID := mustAtoi(parts[0])
				tmp_data.deckBlackLinks[deckID] = append(tmp_data.deckBlackLinks[deckID], mustAtoi(parts[1]))
			case "deck_white":
				if len(parts) < 2 {
					err = fmt.Errorf("invalid deck white row: %q", line)
					return
				}
				deckID := mustAtoi(parts[0])
				tmp_data.deckWhiteLinks[deckID] = append(tmp_data.deckWhiteLinks[deckID], mustAtoi(parts[1]))
			}
		}
		if err == nil {
			if err = scanner.Err(); err == nil {
				data = tmp_data
			}
		}
	}
	return
}

func writeAssets(outDir string, data *sqlData) (err error) {
	if data != nil {
		usedBlack := make(map[int]struct{})
		usedWhite := make(map[int]struct{})
		type deckOut struct {
			dir   string
			meta  deck.DeckMetadata
			black []string
			white []string
		}
		var decks []deckOut
		seenDeckIDs := map[string]int{}
		for legacyID, record := range data.decks {
			if !record.active {
				continue
			}
			meta := record.meta
			if prev, ok := seenDeckIDs[meta.ID]; ok {
				meta.ID = fmt.Sprintf("%s-%d", meta.ID, legacyID)
				if prev == legacyID {
					err = fmt.Errorf("duplicate deck id %q", meta.ID)
					return
				}
			}
			seenDeckIDs[meta.ID] = legacyID
			var blackIDs, whiteIDs []string
			for _, cardID := range data.deckBlackLinks[legacyID] {
				card, ok := data.blackCards[cardID]
				if !ok {
					err = fmt.Errorf("deck %s references unknown black card %d", meta.ID, cardID)
					return
				}
				usedBlack[cardID] = struct{}{}
				blackIDs = append(blackIDs, card.ID)
			}
			for _, cardID := range data.deckWhiteLinks[legacyID] {
				card, ok := data.whiteCards[cardID]
				if !ok {
					err = fmt.Errorf("deck %s references unknown white card %d", meta.ID, cardID)
					return
				}
				usedWhite[cardID] = struct{}{}
				whiteIDs = append(whiteIDs, card.ID)
			}
			slices.Sort(blackIDs)
			slices.Sort(whiteIDs)
			decks = append(decks, deckOut{
				dir:   filepath.Join(outDir, "decks", meta.ID),
				meta:  meta,
				black: uniqueStrings(blackIDs),
				white: uniqueStrings(whiteIDs),
			})
		}
		slices.SortFunc(decks, func(a, b deckOut) (result int) { result = strings.Compare(a.meta.ID, b.meta.ID); return })

		for _, dir := range []string{
			filepath.Join(outDir, "cards", "black"),
			filepath.Join(outDir, "cards", "white"),
			filepath.Join(outDir, "decks"),
		} {
			if err = os.MkdirAll(dir, 0o755); err != nil {
				return
			}
		}
		for cardID := range usedBlack {
			card := data.blackCards[cardID]
			if err = writeJSON(filepath.Join(outDir, "cards", "black", card.ID+".json"), card); err != nil {
				return
			}
		}
		for cardID := range usedWhite {
			card := data.whiteCards[cardID]
			if err = writeJSON(filepath.Join(outDir, "cards", "white", card.ID+".json"), card); err != nil {
				return
			}
		}
		for _, d := range decks {
			if err = os.MkdirAll(d.dir, 0o755); err != nil {
				return
			}
			if err = writeJSON(filepath.Join(d.dir, "deck.json"), d.meta); err != nil {
				return
			}
			if err = writeJSON(filepath.Join(d.dir, "black.json"), d.black); err != nil {
				return
			}
			if err = writeJSON(filepath.Join(d.dir, "white.json"), d.white); err != nil {
				return
			}
		}
	}
	return
}

func writeJSON(path string, value any) (err error) {
	var data []byte
	if data, err = json.MarshalIndent(value, "", "  "); err == nil {
		data = append(data, '\n')
		err = os.WriteFile(path, data, 0o644)
	}
	return
}

func slugify(s string) (result string) {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	result = strings.Trim(b.String(), "-")
	result = strings.ReplaceAll(result, "--", "-")
	return
}

func uniqueStrings(values []string) []string {
	slices.Sort(values)
	return slices.Compact(values)
}

func mustAtoi(s string) (result int) {
	for _, r := range s {
		result = (result * 10) + int(r-'0')
	}
	return
}

package game

import (
	"testing"
	"testing/fstest"

	"github.com/linkdata/xyzzy/internal/deck"
)

func testCatalog(t *testing.T) *deck.Catalog {
	t.Helper()
	fsys := fstest.MapFS{}
	for i := 1; i <= 60; i++ {
		pick := 1
		draw := 0
		if i == 2 {
			pick = 2
		}
		if i == 3 {
			draw = 1
		}
		fsys["assets/cards/black/b"+itoa(i)+".json"] = &fstest.MapFile{Data: []byte(`{"id":"b` + itoa(i) + `","text":"Q` + itoa(i) + `","pick":` + itoa(pick) + `,"draw":` + itoa(draw) + `}`)}
	}
	for i := 1; i <= 80; i++ {
		fsys["assets/cards/white/w"+itoa(i)+".json"] = &fstest.MapFile{Data: []byte(`{"id":"w` + itoa(i) + `","text":"A` + itoa(i) + `"}`)}
	}
	fsys["assets/decks/base/deck.json"] = &fstest.MapFile{Data: []byte(`{"id":"base","name":"Base","enabled_by_default":true}`)}
	fsys["assets/decks/base/black.json"] = &fstest.MapFile{Data: []byte(`["b1","b2","b3","b4","b5","b6","b7","b8","b9","b10","b11","b12","b13","b14","b15","b16","b17","b18","b19","b20","b21","b22","b23","b24","b25","b26","b27","b28","b29","b30","b31","b32","b33","b34","b35","b36","b37","b38","b39","b40","b41","b42","b43","b44","b45","b46","b47","b48","b49","b50"]`)}
	fsys["assets/decks/base/white.json"] = &fstest.MapFile{Data: []byte(`["w1","w2","w3","w4","w5","w6","w7","w8","w9","w10","w11","w12","w13","w14","w15","w16","w17","w18","w19","w20","w21","w22","w23","w24","w25","w26","w27","w28","w29","w30","w31","w32","w33","w34","w35","w36","w37","w38","w39","w40","w41","w42","w43","w44","w45","w46","w47","w48","w49","w50","w51","w52","w53","w54","w55","w56","w57","w58","w59","w60"]`)}
	fsys["assets/decks/expansion/deck.json"] = &fstest.MapFile{Data: []byte(`{"id":"expansion","name":"Expansion"}`)}
	fsys["assets/decks/expansion/black.json"] = &fstest.MapFile{Data: []byte(`["b41","b42","b43","b44","b45","b46","b47","b48","b49","b50","b51","b52","b53","b54","b55","b56","b57","b58","b59","b60"]`)}
	fsys["assets/decks/expansion/white.json"] = &fstest.MapFile{Data: []byte(`["w21","w22","w23","w24","w25","w41","w42","w43","w44","w45","w46","w47","w48","w49","w50","w51","w52","w53","w54","w55","w56","w57","w58","w59","w60","w61","w62","w63","w64","w65","w66","w67","w68","w69","w70","w71","w72","w73","w74","w75","w76","w77","w78","w79","w80"]`)}
	catalog, err := deck.LoadFS(fsys)
	if err != nil {
		t.Fatalf("LoadFS() error = %v", err)
	}
	return catalog
}

func forceRound(t *testing.T, room *Room, blackID string) {
	t.Helper()
	black := room.catalog.BlackCards[blackID]
	if black == nil {
		t.Fatalf("unknown black card %q", blackID)
	}
	room.mu.Lock()
	defer room.mu.Unlock()
	room.blackDraw = []*deck.BlackCard{black}
	room.blackDiscard = nil
	room.currentBlack = nil
	room.submissions = nil
	room.submissionSeq = 0
	room.czarIndex = -1
	room.round = 0
	room.statusMessage = ""
	for _, player := range room.players {
		player.Score = 0
		player.Submitted = nil
		player.Hand = nil
	}
	room.advanceRoundLocked()
}

func testPlayer(name string) *Player {
	name = NormalizeNickname(name)
	return &Player{
		Nickname:      name,
		NicknameInput: name,
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for v > 0 {
		i--
		digits[i] = byte('0' + v%10)
		v /= 10
	}
	return string(digits[i:])
}

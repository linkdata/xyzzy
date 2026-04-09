package ui

import (
	"testing"

	"github.com/linkdata/xyzzy/internal/game"
)

func TestApplyCardSelectionReplacesSinglePickSelection(t *testing.T) {
	page := &RoomPage{SelectedCardIDs: []string{"w1"}}

	page.applyCardSelection("w2", 1)

	if len(page.SelectedCardIDs) != 1 || page.SelectedCardIDs[0] != "w2" {
		t.Fatalf("SelectedCardIDs = %v, want [w2]", page.SelectedCardIDs)
	}
	if page.Alert != "" {
		t.Fatalf("Alert = %q, want empty", page.Alert)
	}
}

func TestApplyCardSelectionKeepsMultiPickLimit(t *testing.T) {
	page := &RoomPage{SelectedCardIDs: []string{"w1", "w2"}}

	page.applyCardSelection("w3", 2)

	if len(page.SelectedCardIDs) != 2 || page.SelectedCardIDs[0] != "w1" || page.SelectedCardIDs[1] != "w2" {
		t.Fatalf("SelectedCardIDs = %v, want unchanged", page.SelectedCardIDs)
	}
	if page.Alert == "" {
		t.Fatal("Alert = empty, want validation message")
	}
}

func TestWaitingTitleForJudgeDuringPlay(t *testing.T) {
	snap := game.RoomView{
		State: game.StatePlaying,
		Players: []game.PlayerView{
			{ID: "p1", IsJudge: true},
			{ID: "p2"},
		},
	}

	if got := waitingTitle(snap, "p1"); got != "Waiting for answers" {
		t.Fatalf("waitingTitle() = %q", got)
	}
	if got := waitingDetail(snap, "p1"); got != "You'll choose the winner once every answer is in." {
		t.Fatalf("waitingDetail() = %q", got)
	}
}

func TestWaitingTitleForSubmittedPlayerDuringPlay(t *testing.T) {
	snap := game.RoomView{
		State: game.StatePlaying,
		Players: []game.PlayerView{
			{ID: "p1", Submitted: true},
			{ID: "p2"},
		},
	}

	if got := waitingTitle(snap, "p1"); got != "Waiting for the rest of the table" {
		t.Fatalf("waitingTitle() = %q", got)
	}
	if got := waitingDetail(snap, "p1"); got != "Your cards are in." {
		t.Fatalf("waitingDetail() = %q", got)
	}
}

func TestWaitingTitleDuringJudging(t *testing.T) {
	snap := game.RoomView{
		State:     game.StateJudging,
		JudgeName: "Casey",
	}

	if got := waitingTitle(snap, "p1"); got != "Casey is picking the winner" {
		t.Fatalf("waitingTitle() = %q", got)
	}
	if got := waitingDetail(snap, "p1"); got != "" {
		t.Fatalf("waitingDetail() = %q", got)
	}
}

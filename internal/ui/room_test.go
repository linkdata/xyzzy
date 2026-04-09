package ui

import "testing"

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

package game

import "testing"

func TestPickTwoRoundAndJudgeFlow(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	room, _ := mgr.CreateRoom("p1", "Alice", []string{"base", "expansion"})
	_, _ = mgr.JoinRoom(room.Code(), "p2", "Bob")
	_, _ = mgr.JoinRoom(room.Code(), "p3", "Casey")
	_, _ = mgr.JoinRoom(room.Code(), "p4", "Drew")
	if err := room.Start("p1"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	forceRound(t, room, "b2")

	var judgeID string
	for _, player := range room.Snapshot("p1").Players {
		if player.IsJudge {
			judgeID = player.ID
		}
	}
	for _, player := range room.Snapshot("p1").Players {
		if player.ID == judgeID {
			continue
		}
		playerSnap := room.Snapshot(player.ID)
		cardIDs := []string{playerSnap.Hand[0].ID, playerSnap.Hand[1].ID}
		if err := room.PlayCards(player.ID, cardIDs); err != nil {
			t.Fatalf("PlayCards(%s) error = %v", player.ID, err)
		}
	}
	judgeSnap := room.Snapshot(judgeID)
	if !judgeSnap.CanJudge || judgeSnap.State != StateJudging || len(judgeSnap.Submissions) != 3 {
		t.Fatalf("judge snapshot = %#v", judgeSnap)
	}
	if err := room.Judge(judgeID, judgeSnap.Submissions[0].ID); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if room.Snapshot("p1").State != StatePlaying {
		t.Fatalf("expected next round to start, got %s", room.Snapshot("p1").State)
	}
}

func TestDrawCardRoundDealsExtraCards(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	room, _ := mgr.CreateRoom("p1", "Alice", []string{"base", "expansion"})
	_, _ = mgr.JoinRoom(room.Code(), "p2", "Bob")
	_, _ = mgr.JoinRoom(room.Code(), "p3", "Casey")
	if err := room.Start("p1"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	forceRound(t, room, "b3")
	snap := room.Snapshot("p1")
	if snap.NeedDraw != 1 {
		t.Fatalf("expected draw=1, got %d", snap.NeedDraw)
	}
	for _, player := range snap.Players {
		playerSnap := room.Snapshot(player.ID)
		if player.IsJudge {
			if len(playerSnap.Hand) != HandSize {
				t.Fatalf("judge hand size = %d, want %d", len(playerSnap.Hand), HandSize)
			}
			continue
		}
		if len(playerSnap.Hand) != HandSize+1 {
			t.Fatalf("non-judge hand size = %d, want %d", len(playerSnap.Hand), HandSize+1)
		}
	}
}

func TestRoomResetOnTooFewPlayers(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	room, _ := mgr.CreateRoom("p1", "Alice", []string{"base", "expansion"})
	_, _ = mgr.JoinRoom(room.Code(), "p2", "Bob")
	_, _ = mgr.JoinRoom(room.Code(), "p3", "Casey")
	if err := room.Start("p1"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	mgr.LeaveRoom(room.Code(), "p3")
	snap := room.Snapshot("p1")
	if snap.State != StateLobby {
		t.Fatalf("expected lobby reset, got %s", snap.State)
	}
	if snap.StatusMessage == "" {
		t.Fatal("expected reset message")
	}
}

func TestFinishedGameResultsPersistInLobby(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	room, _ := mgr.CreateRoom("p1", "Alice", []string{"base", "expansion"})
	_, _ = mgr.JoinRoom(room.Code(), "p2", "Bob")
	_, _ = mgr.JoinRoom(room.Code(), "p3", "Casey")
	if err := room.Start("p1"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	var judgeID string
	var winner *Player
	room.mu.Lock()
	if judge := room.judgeLocked(); judge != nil {
		judgeID = judge.ID
	}
	for _, player := range room.players {
		if player.ID != judgeID {
			winner = player
			break
		}
	}
	if winner == nil {
		room.mu.Unlock()
		t.Fatal("expected non-judge winner candidate")
	}
	winner.Score = ScoreGoal - 1
	room.state = StateJudging
	room.submissions = []*Submission{{
		ID:       "w1",
		PlayerID: winner.ID,
		CardIDs:  []string{"w1"},
	}}
	room.mu.Unlock()

	if err := room.Judge(judgeID, "w1"); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	snap := room.Snapshot("p1")
	if snap.State != StateLobby {
		t.Fatalf("expected lobby reset, got %s", snap.State)
	}
	if snap.LastGameWinner != winner.Nickname {
		t.Fatalf("LastGameWinner = %q, want %q", snap.LastGameWinner, winner.Nickname)
	}
	if len(snap.LastGameScores) != 3 {
		t.Fatalf("LastGameScores = %#v", snap.LastGameScores)
	}
	if !snap.LastGameScores[0].IsWinner || snap.LastGameScores[0].Nickname != winner.Nickname || snap.LastGameScores[0].Score != ScoreGoal {
		t.Fatalf("unexpected winning score row: %#v", snap.LastGameScores[0])
	}
	for _, player := range snap.Players {
		if player.Score != 0 {
			t.Fatalf("player %s score = %d, want reset to 0", player.Nickname, player.Score)
		}
	}
	if err := room.Start("p1"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	nextSnap := room.Snapshot("p1")
	if nextSnap.LastGameWinner != "" || len(nextSnap.LastGameScores) != 0 {
		t.Fatalf("expected last game results to clear on restart, got %#v", nextSnap.LastGameScores)
	}
}

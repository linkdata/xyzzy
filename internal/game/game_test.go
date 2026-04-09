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

func TestManagerRoomLifecycle(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	room, err := mgr.CreateRoom("p1", "Alice", catalog.DefaultDeckIDs())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := mgr.JoinRoom(room.Code(), "p2", "Bob"); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if _, err := mgr.JoinRoom(room.Code(), "p3", "Casey"); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if err := room.Start("p1"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	view := room.Snapshot("p2")
	if !view.InRoom || view.State != StatePlaying {
		t.Fatalf("Snapshot() = %#v", view)
	}
	if len(view.Hand) < HandSize {
		t.Fatalf("expected hand size >= %d, got %d", HandSize, len(view.Hand))
	}
	if view.CurrentBlack == nil {
		t.Fatal("expected current black card")
	}
}

func TestDebugMinPlayersAllowsTwoPlayerStart(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManagerWithOptions(catalog, Options{MinPlayers: 2})
	room, err := mgr.CreateRoom("p1", "Alice", []string{"base", "expansion"})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := mgr.JoinRoom(room.Code(), "p2", "Bob"); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if err := room.Start("p1"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if snap := room.Snapshot("p1"); snap.State != StatePlaying || len(snap.Players) != 2 || snap.CurrentBlack == nil {
		t.Fatalf("unexpected two-player snapshot: %#v", snap)
	}
}

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

func playOutRound(t *testing.T, room *Room) {
	t.Helper()
	snap := room.Snapshot("p1")
	var judgeID string
	for _, player := range snap.Players {
		if player.IsJudge {
			judgeID = player.ID
		}
	}
	for _, player := range snap.Players {
		if player.ID == judgeID {
			continue
		}
		playerSnap := room.Snapshot(player.ID)
		cardIDs := []string{playerSnap.Hand[0].ID}
		for len(cardIDs) < playerSnap.NeedPick {
			cardIDs = append(cardIDs, playerSnap.Hand[len(cardIDs)].ID)
		}
		if err := room.PlayCards(player.ID, cardIDs); err != nil {
			t.Fatalf("PlayCards(%s) error = %v", player.ID, err)
		}
	}
	judgeSnap := room.Snapshot(judgeID)
	if len(judgeSnap.Submissions) == 0 {
		t.Fatal("expected submissions for judge")
	}
	if err := room.Judge(judgeID, judgeSnap.Submissions[0].ID); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
}

func forceRound(t *testing.T, room *Room, blackID string) {
	t.Helper()
	room.mu.Lock()
	defer room.mu.Unlock()
	room.blackDraw = []string{blackID}
	room.blackDiscard = nil
	room.currentBlackID = ""
	room.submissions = nil
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

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + (v % 10))
		v /= 10
	}
	return string(buf[i:])
}

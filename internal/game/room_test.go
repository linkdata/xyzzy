package game

import "testing"

func TestPickTwoRoundAndJudgeFlow(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")
	drew := testPlayer("Drew")

	room, _ := mgr.CreateRoom(alice, []string{"base", "expansion"})
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	_, _ = mgr.JoinRoom(room.Code(), drew)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	forceRound(t, room, "b2")

	judge := room.JudgePlayer()
	if judge == nil {
		t.Fatal("expected judge")
	}
	for _, player := range room.Players() {
		if player == judge {
			continue
		}
		hand := room.HandFor(player)
		cardIDs := []string{hand[0].ID, hand[1].ID}
		if err := room.PlayCards(player, cardIDs); err != nil {
			t.Fatalf("PlayCards(%s) error = %v", player.Nickname, err)
		}
	}

	if !room.CanJudge(judge) || room.State() != StateJudging || len(room.Submissions()) != 3 {
		t.Fatalf("judge state did not advance to judging")
	}
	if err := room.Judge(judge, room.Submissions()[0]); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if room.State() != StatePlaying {
		t.Fatalf("expected next round to start, got %s", room.State())
	}
}

func TestDrawCardRoundDealsExtraCards(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")

	room, _ := mgr.CreateRoom(alice, []string{"base", "expansion"})
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	forceRound(t, room, "b3")
	if room.NeedDraw() != 1 {
		t.Fatalf("NeedDraw() = %d, want 1", room.NeedDraw())
	}
	for _, player := range room.Players() {
		hand := room.HandFor(player)
		if room.IsJudge(player) {
			if len(hand) != HandSize {
				t.Fatalf("judge hand size = %d, want %d", len(hand), HandSize)
			}
			continue
		}
		if len(hand) != HandSize+1 {
			t.Fatalf("non-judge hand size = %d, want %d", len(hand), HandSize+1)
		}
	}
}

func TestRoomResetOnTooFewPlayers(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")

	room, _ := mgr.CreateRoom(alice, []string{"base", "expansion"})
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	mgr.LeaveRoom(casey)

	if room.State() != StateLobby {
		t.Fatalf("expected lobby reset, got %s", room.State())
	}
	if room.StatusMessage() == "" {
		t.Fatal("expected reset message")
	}
}

func TestJoinDuringPlayingDealsCurrentRoundHandAndAllowsSubmission(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")
	drew := testPlayer("Drew")

	room, _ := mgr.CreateRoom(alice, []string{"base", "expansion"})
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	forceRound(t, room, "b3")

	if _, err := mgr.JoinRoom(room.Code(), drew); err != nil {
		t.Fatalf("JoinRoom() during playing error = %v", err)
	}
	if drew.Room != room {
		t.Fatal("expected joining player to be seated in playing room")
	}
	if got := len(room.HandFor(drew)); got != HandSize+1 {
		t.Fatalf("joined player hand size = %d, want %d", got, HandSize+1)
	}
	if !room.CanSubmit(drew) {
		t.Fatal("expected joined player to be able to submit in the current round")
	}

	judge := room.JudgePlayer()
	if judge == nil {
		t.Fatal("expected judge")
	}
	for _, player := range room.Players() {
		if player == judge {
			continue
		}
		hand := room.HandFor(player)
		cardIDs := []string{hand[0].ID}
		if err := room.PlayCards(player, cardIDs); err != nil {
			t.Fatalf("PlayCards(%s) error = %v", player.Nickname, err)
		}
	}
	if room.State() != StateJudging || len(room.Submissions()) != len(room.Players())-1 {
		t.Fatalf("expected joined player submission to count toward judging transition")
	}
}

func TestJoinDuringJudgingWaitsForNextRound(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")
	drew := testPlayer("Drew")

	room, _ := mgr.CreateRoom(alice, []string{"base", "expansion"})
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	judge := room.JudgePlayer()
	if judge == nil {
		t.Fatal("expected judge")
	}
	for _, player := range room.Players() {
		if player == judge {
			continue
		}
		hand := room.HandFor(player)
		if err := room.PlayCards(player, []string{hand[0].ID}); err != nil {
			t.Fatalf("PlayCards(%s) error = %v", player.Nickname, err)
		}
	}
	if room.State() != StateJudging {
		t.Fatalf("expected judging state, got %s", room.State())
	}

	if _, err := mgr.JoinRoom(room.Code(), drew); err != nil {
		t.Fatalf("JoinRoom() during judging error = %v", err)
	}
	if got := len(room.HandFor(drew)); got != HandSize {
		t.Fatalf("joined player hand size during judging = %d, want %d", got, HandSize)
	}
	if room.CanSubmit(drew) {
		t.Fatal("joined player should wait until the next round during judging")
	}

	if err := room.Judge(judge, room.Submissions()[0]); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if room.State() != StatePlaying {
		t.Fatalf("expected next round after judging, got %s", room.State())
	}
	if !room.CanSubmit(drew) {
		t.Fatal("joined player should be active next round")
	}
}

func TestJoinDuringGameRequiresEnoughCardsForAnotherPlayer(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")
	drew := testPlayer("Drew")

	room, _ := mgr.CreateRoom(alice, []string{"base"})
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if room.CanJoin(drew) {
		t.Fatal("expected in-progress room with too few white cards to reject another player")
	}
	if _, err := mgr.JoinRoom(room.Code(), drew); err != ErrNotEnoughWhiteCards {
		t.Fatalf("JoinRoom() error = %v, want %v", err, ErrNotEnoughWhiteCards)
	}
}

func TestJudgeLeavingResetsToLobbyAndHostLeavingReassigns(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")

	room, _ := mgr.CreateRoom(alice, []string{"base", "expansion"})
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	judge := room.JudgePlayer()
	if judge == nil {
		t.Fatal("expected judge")
	}
	mgr.LeaveRoom(judge)

	if room.State() != StateLobby {
		t.Fatalf("expected judge leaving to reset lobby, got %s", room.State())
	}

	hostBefore := room.Host()
	if hostBefore == nil {
		t.Fatal("expected host after judge leave")
	}
	mgr.LeaveRoom(hostBefore)
	if room.Host() == hostBefore {
		t.Fatal("expected host reassignment")
	}
}

func TestFinishedGameResultsPersistInLobby(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")

	room, _ := mgr.CreateRoom(alice, []string{"base", "expansion"})
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	var winner *Player
	room.mu.Lock()
	judge := room.judgeLocked()
	for _, player := range room.players {
		if player != judge {
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
		ID:      "w1",
		Player:  winner,
		CardIDs: []string{"w1"},
	}}
	room.mu.Unlock()

	if err := room.Judge(judge, room.Submissions()[0]); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if room.State() != StateLobby {
		t.Fatalf("expected lobby reset, got %s", room.State())
	}
	if room.LastGameWinner() != winner.Nickname {
		t.Fatalf("LastGameWinner() = %q, want %q", room.LastGameWinner(), winner.Nickname)
	}
	if len(room.LastGameScores()) != 3 {
		t.Fatalf("LastGameScores() = %#v", room.LastGameScores())
	}
	if !room.LastGameScores()[0].IsWinner || room.LastGameScores()[0].Nickname != winner.Nickname || room.LastGameScores()[0].Score != ScoreGoal {
		t.Fatalf("unexpected winning score row: %#v", room.LastGameScores()[0])
	}
	for _, player := range room.Players() {
		if room.ScoreFor(player) != 0 {
			t.Fatalf("player %s score = %d, want reset to 0", player.Nickname, room.ScoreFor(player))
		}
	}
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if room.LastGameWinner() != "" || len(room.LastGameScores()) != 0 {
		t.Fatalf("expected last game results to clear on restart, got %#v", room.LastGameScores())
	}
}

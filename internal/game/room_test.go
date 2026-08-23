package game

import (
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/linkdata/xyzzy/internal/deck"
)

func TestPickTwoRoundAndJudgeFlow(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")
	drew := testPlayer("Drew")

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
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
		cards := []*deck.WhiteCard{hand[0], hand[1]}
		if err := room.PlayCards(player, cards); err != nil {
			t.Fatalf("PlayCards(%s) error = %v", player.Nickname, err)
		}
	}

	if !room.CanJudge(judge) || room.State() != StateJudging || len(room.Submissions()) != 3 {
		t.Fatalf("judge state did not advance to judging")
	}
	winningSubmission := room.Submissions()[0]
	if err := room.Judge(judge, winningSubmission); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if room.State() != StateReview {
		t.Fatalf("expected round review state, got %s", room.State())
	}
	if !room.IsRoundWinner(winningSubmission.Player) || !room.IsWinningSubmission(winningSubmission) {
		t.Fatal("expected winning player and submission to be marked during round review")
	}
	if err := room.ProceedReview(judge); err != nil {
		t.Fatalf("ProceedReview() error = %v", err)
	}
	if room.State() != StatePlaying {
		t.Fatalf("expected next round to start after proceed, got %s", room.State())
	}
}

func TestDrawCardRoundDealsExtraCards(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	forceRound(t, room, "b3")
	if black := room.CurrentBlack(); black == nil || black.Draw != 1 {
		t.Fatalf("CurrentBlack() = %#v, want draw 1", black)
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

func TestStartRejectsGameInProgress(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := room.Start(alice); !errors.Is(err, ErrGameInProgress) {
		t.Fatalf("Start() while playing error = %v, want %v", err, ErrGameInProgress)
	}
	if room.State() != StatePlaying {
		t.Fatalf("State() = %s, want %s", room.State(), StatePlaying)
	}
}

func TestStartClearsStaleSelections(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	alice.SelectedSubmission = &Submission{ID: "stale"}
	bob.SelectedCards = []*deck.WhiteCard{catalog.WhiteCards["w1"]}
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	for _, player := range []*Player{alice, bob, casey} {
		if len(player.SelectedCards) != 0 || player.SelectedSubmission != nil {
			t.Fatalf("%s selections after Start() = cards %#v submission %#v, want cleared", player.Nickname, player.SelectedCards, player.SelectedSubmission)
		}
	}
}

func TestSubmissionIDsUseRoundSequence(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	forceRound(t, room, "b1")

	judge := room.JudgePlayer()
	if judge == nil {
		t.Fatal("expected judge")
	}

	var nonJudge []*Player
	for _, player := range room.Players() {
		if player != judge {
			nonJudge = append(nonJudge, player)
		}
	}
	if len(nonJudge) != 2 {
		t.Fatalf("expected 2 non-judge players, got %d", len(nonJudge))
	}

	firstHand := room.HandFor(nonJudge[0])
	if err := room.PlayCards(nonJudge[0], []*deck.WhiteCard{firstHand[0]}); err != nil {
		t.Fatalf("PlayCards(first) error = %v", err)
	}
	if got := room.Submissions(); len(got) != 1 || got[0].ID != "r1-s1" {
		t.Fatalf("first submission IDs = %#v, want [r1-s1]", got)
	}

	secondHand := room.HandFor(nonJudge[1])
	if err := room.PlayCards(nonJudge[1], []*deck.WhiteCard{secondHand[0]}); err != nil {
		t.Fatalf("PlayCards(second) error = %v", err)
	}
	if room.State() != StateJudging {
		t.Fatalf("State() = %s, want %s", room.State(), StateJudging)
	}
	seen := map[string]bool{}
	for _, submission := range room.Submissions() {
		seen[submission.ID] = true
	}
	if !seen["r1-s1"] || !seen["r1-s2"] || len(seen) != 2 {
		t.Fatalf("submission IDs after round 1 = %#v, want r1-s1 and r1-s2", seen)
	}

	if err := room.Judge(judge, room.Submissions()[0]); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if err := room.ProceedReview(judge); err != nil {
		t.Fatalf("ProceedReview() error = %v", err)
	}
	if room.State() != StatePlaying {
		t.Fatalf("State() = %s, want %s", room.State(), StatePlaying)
	}

	judge = room.JudgePlayer()
	if judge == nil {
		t.Fatal("expected judge for round 2")
	}
	var player *Player
	for _, p := range room.Players() {
		if p != judge {
			player = p
			break
		}
	}
	hand := room.HandFor(player)
	if err := room.PlayCards(player, []*deck.WhiteCard{hand[0]}); err != nil {
		t.Fatalf("PlayCards(round2) error = %v", err)
	}
	if got := room.Submissions(); len(got) != 1 || got[0].ID != "r2-s1" {
		t.Fatalf("first submission IDs in round 2 = %#v, want [r2-s1]", got)
	}
}

func TestRoomResetOnTooFewPlayers(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	mgr.LeaveRoom(casey)

	if room.State() != StateLobby {
		t.Fatalf("expected lobby reset, got %s", room.State())
	}
}

func TestJoinDuringPlayingDealsCurrentRoundHandAndAllowsSubmission(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")
	drew := testPlayer("Drew")

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	forceRound(t, room, "b3")

	if _, err := mgr.JoinRoom(room.Code(), drew); err != nil {
		t.Fatalf("JoinRoom() during playing error = %v", err)
	}
	if drew.Room() != room {
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
		cards := []*deck.WhiteCard{hand[0]}
		if err := room.PlayCards(player, cards); err != nil {
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

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	forceRound(t, room, "b1")

	judge := room.JudgePlayer()
	if judge == nil {
		t.Fatal("expected judge")
	}
	for _, player := range room.Players() {
		if player == judge {
			continue
		}
		hand := room.HandFor(player)
		needPick := room.NeedPick()
		if len(hand) < needPick {
			t.Fatalf("hand size for %s = %d, need at least %d", player.Nickname, len(hand), needPick)
		}
		if err := room.PlayCards(player, hand[:needPick]); err != nil {
			black := room.CurrentBlack()
			blackID := "<nil>"
			blackPick := -1
			if black != nil {
				blackID = black.ID
				blackPick = black.Pick
			}
			t.Fatalf(
				"PlayCards(%s) error = %v (needPick=%d hand=%d submitted=%d state=%s black=%s pick=%d)",
				player.Nickname, err, needPick, len(hand), len(room.Submissions()), room.State(), blackID, blackPick,
			)
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
	if room.State() != StateReview {
		t.Fatalf("expected review state after judging, got %s", room.State())
	}
	if err := room.ProceedReview(judge); err != nil {
		t.Fatalf("ProceedReview() error = %v", err)
	}
	if room.State() != StatePlaying {
		t.Fatalf("expected next round after proceed, got %s", room.State())
	}
	if !room.CanSubmit(drew) {
		t.Fatal("joined player should be active next round")
	}
}

func TestRoundReviewAutoAdvancesAfterDelay(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
	room.reviewDelay = 10 * time.Millisecond
	_, _ = mgr.JoinRoom(room.Code(), bob)
	_, _ = mgr.JoinRoom(room.Code(), casey)
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	forceRound(t, room, "b1")

	judge := room.JudgePlayer()
	if judge == nil {
		t.Fatal("expected judge")
	}
	for _, player := range room.Players() {
		if player == judge {
			continue
		}
		hand := room.HandFor(player)
		needPick := room.NeedPick()
		if len(hand) < needPick {
			t.Fatalf("hand size for %s = %d, need at least %d", player.Nickname, len(hand), needPick)
		}
		if err := room.PlayCards(player, hand[:needPick]); err != nil {
			t.Fatalf("PlayCards(%s) error = %v", player.Nickname, err)
		}
	}
	if err := room.Judge(judge, room.Submissions()[0]); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if room.State() != StateReview {
		t.Fatalf("expected review state after judge pick, got %s", room.State())
	}

	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if room.State() == StatePlaying {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected review timer to auto-advance, got %s", room.State())
}

func TestReviewTimerUpdatesCountdownAndAdvances(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var notificationsMu sync.Mutex
		var notifications [][]any
		manager := NewManagerWithOptions(nil, Options{
			Dirty: func(tags ...any) {
				notificationsMu.Lock()
				notifications = append(notifications, append([]any(nil), tags...))
				notificationsMu.Unlock()
			},
		})
		notificationSnapshot := func() (result [][]any) {
			notificationsMu.Lock()
			defer notificationsMu.Unlock()
			result = append([][]any(nil), notifications...)
			return
		}
		judge := testPlayer("Judge")
		winner := testPlayer("Winner")
		room := &Room{
			manager:     manager,
			state:       StateJudging,
			players:     []*Player{judge, winner},
			host:        judge,
			czarIndex:   0,
			reviewDelay: 2500 * time.Millisecond,
		}

		room.mu.Lock()
		room.beginReviewLocked(winner, nil, true)
		room.mu.Unlock()
		countdown := room.ReviewStatus(winner)
		if got := countdown.JawsGet(nil); got != "Returning to the lobby in 3 seconds." {
			t.Fatalf("initial Countdown = %q", got)
		}

		synctest.Wait()
		if got := notificationSnapshot(); len(got) != 0 {
			t.Fatalf("notifications before first boundary = %#v", got)
		}

		time.Sleep(500 * time.Millisecond)
		synctest.Wait()
		if got := countdown.JawsGet(nil); got != "Returning to the lobby in 2 seconds." {
			t.Fatalf("Countdown after first boundary = %q", got)
		}
		gotNotifications := notificationSnapshot()
		if len(gotNotifications) != 1 || len(gotNotifications[0]) != 1 || gotNotifications[0][0] != &room.reviewDeadline {
			t.Fatalf("notifications after first boundary = %#v", gotNotifications)
		}

		time.Sleep(time.Second)
		synctest.Wait()
		if got := countdown.JawsGet(nil); got != "Returning to the lobby in 1 second." {
			t.Fatalf("Countdown after second boundary = %q", got)
		}
		gotNotifications = notificationSnapshot()
		if len(gotNotifications) != 2 || len(gotNotifications[1]) != 1 || gotNotifications[1][0] != &room.reviewDeadline {
			t.Fatalf("notifications after second boundary = %#v", gotNotifications)
		}

		time.Sleep(time.Second)
		synctest.Wait()
		if got := room.State(); got != StateLobby {
			t.Fatalf("State() at deadline = %s, want %s", got, StateLobby)
		}
		if got := countdown.JawsGet(nil); got != "" {
			t.Fatalf("Countdown after review = %q, want empty", got)
		}
		gotNotifications = notificationSnapshot()
		if len(gotNotifications) != 3 || len(gotNotifications[2]) != 1 || gotNotifications[2][0] != room {
			t.Fatalf("notifications at deadline = %#v", gotNotifications)
		}
		if room.reviewTimer != nil {
			t.Fatalf("reviewTimer at deadline = %v, want nil", room.reviewTimer)
		}

		time.Sleep(10 * time.Second)
		synctest.Wait()
		if got := notificationSnapshot(); len(got) != 3 {
			t.Fatalf("notifications after review ended = %#v", got)
		}
	})
}

func TestProceedReviewStopsCountdownUpdates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var notificationsMu sync.Mutex
		var notifications int
		manager := NewManagerWithOptions(nil, Options{
			Dirty: func(...any) {
				notificationsMu.Lock()
				notifications++
				notificationsMu.Unlock()
			},
		})
		notificationCount := func() (result int) {
			notificationsMu.Lock()
			result = notifications
			notificationsMu.Unlock()
			return
		}
		judge := testPlayer("Judge")
		winner := testPlayer("Winner")
		room := &Room{
			manager:     manager,
			state:       StateJudging,
			players:     []*Player{judge, winner},
			host:        judge,
			czarIndex:   0,
			reviewDelay: 2500 * time.Millisecond,
		}

		room.mu.Lock()
		room.beginReviewLocked(winner, nil, true)
		room.mu.Unlock()
		time.Sleep(500 * time.Millisecond)
		synctest.Wait()
		if got := notificationCount(); got != 1 {
			t.Fatalf("notification count after first boundary = %d, want 1", got)
		}

		if err := room.ProceedReview(judge); err != nil {
			t.Fatalf("ProceedReview() error = %v", err)
		}
		if room.reviewTimer != nil {
			t.Fatalf("reviewTimer after ProceedReview = %v, want nil", room.reviewTimer)
		}
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if got := notificationCount(); got != 1 {
			t.Fatalf("notification count after ProceedReview = %d, want 1", got)
		}
	})
}

func TestJoinDuringGameRequiresEnoughCardsForAnotherPlayer(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")
	drew := testPlayer("Drew")

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base"))
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

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
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

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
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
		ID:     "r1-s1",
		Player: winner,
		Cards:  []*deck.WhiteCard{catalog.WhiteCards["w1"]},
	}}
	room.mu.Unlock()

	if err := room.Judge(judge, room.Submissions()[0]); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if room.State() != StateReview {
		t.Fatalf("expected review state before lobby reset, got %s", room.State())
	}
	if err := room.ProceedReview(judge); err != nil {
		t.Fatalf("ProceedReview() error = %v", err)
	}
	if room.State() != StateLobby {
		t.Fatalf("expected lobby reset after proceed, got %s", room.State())
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

func TestJudgeSelectedSubmissionClearsWhenAuthorLeaves(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")
	drew := testPlayer("Drew")

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
	for _, player := range []*Player{bob, casey, drew} {
		if _, err := mgr.JoinRoom(room.Code(), player); err != nil {
			t.Fatalf("JoinRoom(%s) error = %v", player.Nickname, err)
		}
	}
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	forceRound(t, room, "b1")

	judge := room.JudgePlayer()
	var submitters []*Player
	for _, player := range room.Players() {
		if player == judge {
			continue
		}
		submitters = append(submitters, player)
		hand := room.HandFor(player)
		if err := room.PlayCards(player, hand[:room.NeedPick()]); err != nil {
			t.Fatalf("PlayCards(%s) error = %v", player.Nickname, err)
		}
	}
	if room.State() != StateJudging {
		t.Fatalf("State() = %s, want %s", room.State(), StateJudging)
	}

	leaver := submitters[0]
	var leaverSubmission *Submission
	for _, sub := range room.Submissions() {
		if sub.Player == leaver {
			leaverSubmission = sub
			break
		}
	}
	if leaverSubmission == nil {
		t.Fatal("expected leaver to have a submission")
	}
	judge.SelectedSubmission = leaverSubmission

	if _, empty := mgr.LeaveRoom(leaver); empty {
		t.Fatal("expected room to remain populated after non-judge leave")
	}
	if room.State() != StateJudging {
		t.Fatalf("expected state to stay judging, got %s", room.State())
	}
	if judge.SelectedSubmission != nil {
		t.Fatalf("expected judge SelectedSubmission to clear after submitter left, got %#v", judge.SelectedSubmission)
	}
}

func TestRoundWinnerLeavingAdvancesReview(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	alice := testPlayer("Alice")
	bob := testPlayer("Bob")
	casey := testPlayer("Casey")
	drew := testPlayer("Drew")

	room, _ := mgr.CreateRoom(alice, testDecks(t, catalog, "base", "expansion"))
	room.reviewDelay = time.Hour
	for _, player := range []*Player{bob, casey, drew} {
		if _, err := mgr.JoinRoom(room.Code(), player); err != nil {
			t.Fatalf("JoinRoom(%s) error = %v", player.Nickname, err)
		}
	}
	if err := room.Start(alice); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	forceRound(t, room, "b1")

	judge := room.JudgePlayer()
	for _, player := range room.Players() {
		if player == judge {
			continue
		}
		hand := room.HandFor(player)
		if err := room.PlayCards(player, hand[:room.NeedPick()]); err != nil {
			t.Fatalf("PlayCards(%s) error = %v", player.Nickname, err)
		}
	}

	winningSubmission := room.Submissions()[0]
	winner := winningSubmission.Player
	if err := room.Judge(judge, winningSubmission); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if room.State() != StateReview {
		t.Fatalf("expected review state, got %s", room.State())
	}

	if _, empty := mgr.LeaveRoom(winner); empty {
		t.Fatal("expected room to remain populated")
	}
	if room.State() != StatePlaying {
		t.Fatalf("expected round to advance after winner leaves, got %s", room.State())
	}
}

func TestSetDeckEnabledReportsChanges(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	host := testPlayer("Alice")
	guest := testPlayer("Bob")

	room, _ := mgr.CreateRoom(host, catalog.DefaultDecks())
	if _, err := mgr.JoinRoom(room.Code(), guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	base := catalog.DeckByID("base")
	if changed, err := room.SetDeckEnabled(guest, base, false); !errors.Is(err, ErrOnlyHostCanEdit) || changed {
		t.Fatalf("SetDeckEnabled(non-host) = (%v, %v), want (false, %v)", changed, err, ErrOnlyHostCanEdit)
	}
	if changed, err := room.SetDeckEnabled(host, base, true); err != nil || changed {
		t.Fatalf("SetDeckEnabled(initial state) = (%v, %v), want (false, nil)", changed, err)
	}
	if changed, err := room.SetDeckEnabled(host, base, false); err != nil || !changed {
		t.Fatalf("SetDeckEnabled(disable) = (%v, %v), want (true, nil)", changed, err)
	}
	if room.DeckEnabled(base) {
		t.Fatal("DeckEnabled(base) after disable = true, want false")
	}
	if changed, err := room.SetDeckEnabled(host, base, false); err != nil || changed {
		t.Fatalf("SetDeckEnabled(repeated disable) = (%v, %v), want (false, nil)", changed, err)
	}
	if changed, err := room.SetDeckEnabled(host, base, true); err != nil || !changed {
		t.Fatalf("SetDeckEnabled(enable) = (%v, %v), want (true, nil)", changed, err)
	}
	if !room.DeckEnabled(base) {
		t.Fatal("DeckEnabled(base) after enable = false, want true")
	}
	room.mu.Lock()
	room.state = StatePlaying
	room.mu.Unlock()
	if changed, err := room.SetDeckEnabled(host, base, false); !errors.Is(err, ErrDecksLocked) || changed {
		t.Fatalf("SetDeckEnabled(in game) = (%v, %v), want (false, %v)", changed, err, ErrDecksLocked)
	}
}

func TestSetDeckEnabledRejectsUnknownDeckPointer(t *testing.T) {
	catalog := testCatalog(t)
	mgr := NewManager(catalog)
	host := testPlayer("Alice")

	room, _ := mgr.CreateRoom(host, catalog.DefaultDecks())
	unknown := &deck.Deck{DeckMetadata: deck.DeckMetadata{ID: "base", Name: "Base copy"}}
	if changed, err := room.SetDeckEnabled(host, unknown, true); !errors.Is(err, ErrUnknownDeck) || changed {
		t.Fatalf("SetDeckEnabled() = (%v, %v), want (false, %v)", changed, err, ErrUnknownDeck)
	}
}

func TestDrawLockedReturnsNilWhenPilesEmpty(t *testing.T) {
	room := &Room{rand: newCryptoRand()}
	if card := room.drawWhiteLocked(); card != nil {
		t.Fatalf("drawWhiteLocked() = %#v, want nil", card)
	}
	if card := room.drawBlackLocked(); card != nil {
		t.Fatalf("drawBlackLocked() = %#v, want nil", card)
	}
}

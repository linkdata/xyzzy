package game

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/jawstest"
	"github.com/linkdata/jaws/lib/tag"
	"github.com/linkdata/xyzzy/internal/deck"
)

func TestNicknameFieldBindsInput(t *testing.T) {
	player := testPlayer("Alice")
	field := player.NicknameField()

	if err := field.JawsSet(nil, "Bob"); err != nil {
		t.Fatalf("JawsSet() error = %v", err)
	}
	if got := player.NicknameInputValue(); got != "Bob" {
		t.Fatalf("NicknameInputValue() = %q, want Bob", got)
	}
	if err := field.JawsSet(nil, "Bob"); !errors.Is(err, jaws.ErrValueUnchanged) {
		t.Fatalf("unchanged JawsSet() error = %v, want %v", err, jaws.ErrValueUnchanged)
	}
}

func TestTargetScoreBinderValidatesNormalizesAndPreservesUnchanged(t *testing.T) {
	catalog := testCatalog(t)
	manager := NewManagerWithOptions(catalog, Options{MinPlayers: 2})
	host := testPlayer("Host")
	guest := testPlayer("Guest")
	room, err := manager.CreateRoom(host, catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err = manager.JoinRoom(room.Code(), guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	elem := newJawsTestElement(t)

	if err = room.TargetScoreBinder(guest).JawsSet(elem, 8); !errors.Is(err, ErrOnlyHostCanEdit) {
		t.Fatalf("guest JawsSet() error = %v, want %v", err, ErrOnlyHostCanEdit)
	}
	if got := room.TargetScore(); got != ScoreGoal {
		t.Fatalf("TargetScore() after rejected edit = %d, want %d", got, ScoreGoal)
	}

	hostBinder := room.TargetScoreBinder(host)
	if err = hostBinder.JawsSet(elem, -1); err != nil {
		t.Fatalf("low JawsSet() error = %v", err)
	}
	if got := room.TargetScore(); got != 2 {
		t.Fatalf("TargetScore() after low edit = %d, want 2", got)
	}
	if err = hostBinder.JawsSet(elem, 99); err != nil {
		t.Fatalf("high JawsSet() error = %v", err)
	}
	if got := room.TargetScore(); got != 10 {
		t.Fatalf("TargetScore() after high edit = %d, want 10", got)
	}
	if err = hostBinder.JawsSet(elem, 10); !errors.Is(err, jaws.ErrValueUnchanged) {
		t.Fatalf("unchanged JawsSet() error = %v, want %v", err, jaws.ErrValueUnchanged)
	}

	room.mu.Lock()
	room.state = StatePlaying
	room.mu.Unlock()
	if err = hostBinder.JawsSet(elem, 7); !errors.Is(err, ErrGameInProgress) {
		t.Fatalf("in-game JawsSet() error = %v, want %v", err, ErrGameInProgress)
	}
}

func TestTargetScoreBinderUsesDebugMinimum(t *testing.T) {
	catalog := testCatalog(t)
	manager := NewManagerWithOptions(catalog, Options{MinPlayers: 2, Debug: true})
	host := testPlayer("Host")
	room, err := manager.CreateRoom(host, catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	if err = room.TargetScoreBinder(host).JawsSet(newJawsTestElement(t), 0); err != nil {
		t.Fatalf("JawsSet() error = %v", err)
	}
	if got := room.TargetScore(); got != 1 {
		t.Fatalf("TargetScore() = %d, want 1", got)
	}
}

func TestSetTargetScoreUsesTheSameValidationAndNormalization(t *testing.T) {
	catalog := testCatalog(t)
	manager := NewManager(catalog)
	host := testPlayer("Host")
	guest := testPlayer("Guest")
	room, err := manager.CreateRoom(host, catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err = manager.JoinRoom(room.Code(), guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}

	if err = room.SetTargetScore(guest, 8); !errors.Is(err, ErrOnlyHostCanEdit) {
		t.Fatalf("SetTargetScore(guest) error = %v, want %v", err, ErrOnlyHostCanEdit)
	}
	if err = room.SetTargetScore(host, 0); err != nil {
		t.Fatalf("SetTargetScore(host) error = %v", err)
	}
	if got := room.TargetScore(); got != 2 {
		t.Fatalf("TargetScore() = %d, want 2", got)
	}
}

func TestPrivateToggleSuccessRunsOnlyForChanges(t *testing.T) {
	catalog := testCatalog(t)
	manager := NewManager(catalog)
	host := testPlayer("Host")
	guest := testPlayer("Guest")
	room, err := manager.CreateRoom(host, catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err = manager.JoinRoom(room.Code(), guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	elem := newJawsTestElement(t)
	successes := 0
	binder := room.PrivateToggle(host).Success(func() {
		successes++
	})

	if err = binder.JawsSet(elem, false); !errors.Is(err, jaws.ErrValueUnchanged) {
		t.Fatalf("initial no-op JawsSet() error = %v, want %v", err, jaws.ErrValueUnchanged)
	}
	if successes != 0 {
		t.Fatalf("success count after no-op = %d, want 0", successes)
	}
	if err = binder.JawsSet(elem, true); err != nil {
		t.Fatalf("changed JawsSet() error = %v", err)
	}
	if !room.IsPrivate() {
		t.Fatal("room is not private after accepted edit")
	}
	if successes != 1 {
		t.Fatalf("success count after change = %d, want 1", successes)
	}
	if err = binder.JawsSet(elem, true); !errors.Is(err, jaws.ErrValueUnchanged) {
		t.Fatalf("second no-op JawsSet() error = %v, want %v", err, jaws.ErrValueUnchanged)
	}
	if successes != 1 {
		t.Fatalf("success count after second no-op = %d, want 1", successes)
	}

	if err = room.PrivateToggle(guest).JawsSet(elem, false); !errors.Is(err, ErrOnlyHostCanEdit) {
		t.Fatalf("guest JawsSet() error = %v, want %v", err, ErrOnlyHostCanEdit)
	}
	room.mu.Lock()
	room.state = StatePlaying
	room.mu.Unlock()
	if err = room.PrivateToggle(host).JawsSet(elem, false); !errors.Is(err, ErrGameInProgress) {
		t.Fatalf("in-game JawsSet() error = %v, want %v", err, ErrGameInProgress)
	}
}

func TestLobbyControlAndButtonInitialAttrs(t *testing.T) {
	catalog := testCatalog(t)
	manager := NewManagerWithOptions(catalog, Options{MinPlayers: 2})
	host := testPlayer("Host")
	guest := testPlayer("Guest")
	room, err := manager.CreateRoom(host, catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	if got := room.LobbyControlAttrs(host); got != "" {
		t.Fatalf("LobbyControlAttrs(host) = %q, want empty", got)
	}
	if got := room.LobbyControlAttrs(guest); got != `disabled` {
		t.Fatalf("LobbyControlAttrs(guest) = %q, want disabled", got)
	}
	if got := room.StartGameButton(host).JawsInitialHTMLAttr(nil); got != `disabled` {
		t.Fatalf("StartGameButton(host) attrs with one player = %q, want disabled", got)
	}
	if got := room.StartGameButton(guest).JawsInitialHTMLAttr(nil); got != `hidden` {
		t.Fatalf("StartGameButton(guest) attrs = %q, want hidden", got)
	}
	if _, err = manager.JoinRoom(room.Code(), guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	if got := room.StartGameButton(host).JawsInitialHTMLAttr(nil); got != "" {
		t.Fatalf("StartGameButton(host) attrs with enough players = %q, want empty", got)
	}
	if got := room.StartGameButton(host).JawsGetHTML(nil); got != "Start Game" {
		t.Fatalf("StartGameButton() content = %q, want Start Game", got)
	}

	room.mu.Lock()
	room.state = StatePlaying
	room.czarIndex = 0
	room.currentBlack = catalog.BlackCards["b1"]
	guest.Hand = []*deck.WhiteCard{catalog.WhiteCards["w1"]}
	room.mu.Unlock()
	if got := room.SubmitCardsButton(guest).JawsInitialHTMLAttr(nil); got != `disabled` {
		t.Fatalf("SubmitCardsButton() attrs without selection = %q, want disabled", got)
	}
	room.mu.Lock()
	guest.SelectedCards = []*deck.WhiteCard{catalog.WhiteCards["w1"]}
	room.mu.Unlock()
	if got := room.SubmitCardsButton(guest).JawsInitialHTMLAttr(nil); got != "" {
		t.Fatalf("SubmitCardsButton() attrs with complete selection = %q, want empty", got)
	}
	if got := room.SubmitCardsButton(guest).JawsGetHTML(nil); got != "Play Selected Cards" {
		t.Fatalf("SubmitCardsButton() content = %q, want Play Selected Cards", got)
	}

	submission := &Submission{ID: "submission", Player: guest}
	room.mu.Lock()
	room.state = StateJudging
	room.submissions = []*Submission{submission}
	host.SelectedSubmission = nil
	room.mu.Unlock()
	if got := room.JudgeButton(host).JawsInitialHTMLAttr(nil); got != `disabled` {
		t.Fatalf("JudgeButton() attrs without selection = %q, want disabled", got)
	}
	room.mu.Lock()
	host.SelectedSubmission = submission
	room.mu.Unlock()
	if got := room.JudgeButton(host).JawsInitialHTMLAttr(nil); got != "" {
		t.Fatalf("JudgeButton() attrs with selection = %q, want empty", got)
	}
	if got := room.JudgeButton(guest).JawsInitialHTMLAttr(nil); got != `disabled` {
		t.Fatalf("JudgeButton(non-judge) attrs = %q, want disabled", got)
	}
	if got := room.JudgeButton(host).JawsGetHTML(nil); got != "Pick Winner" {
		t.Fatalf("JudgeButton() content = %q, want Pick Winner", got)
	}
}

func TestRoomButtonsRunTheirDomainActions(t *testing.T) {
	catalog := testCatalog(t)
	manager := NewManagerWithOptions(catalog, Options{MinPlayers: 2, Debug: true})
	host := testPlayer("Host")
	guest := testPlayer("Guest")
	room, err := manager.CreateRoom(host, testDecks(t, catalog, "base", "expansion"))
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err = manager.JoinRoom(room.Code(), guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}
	elem := newJawsTestElement(t)

	if err = room.StartGameButton(host).JawsClick(elem, jaws.Click{}); err != nil {
		t.Fatalf("StartGameButton().JawsClick() error = %v", err)
	}
	if room.State() != StatePlaying {
		t.Fatalf("State() after start click = %s, want %s", room.State(), StatePlaying)
	}
	forceRound(t, room, "b1")

	judge := room.JudgePlayer()
	contestant := host
	if contestant == judge {
		contestant = guest
	}
	hand := room.HandFor(contestant)
	if len(hand) == 0 || !room.ToggleCardSelection(contestant, hand[0]) {
		t.Fatal("failed to select a card for submission")
	}
	if err = room.SubmitCardsButton(contestant).JawsClick(elem, jaws.Click{}); err != nil {
		t.Fatalf("SubmitCardsButton().JawsClick() error = %v", err)
	}
	if room.State() != StateJudging {
		t.Fatalf("State() after submit click = %s, want %s", room.State(), StateJudging)
	}

	submissions := room.Submissions()
	if len(submissions) != 1 || !room.ToggleSubmissionSelection(judge, submissions[0]) {
		t.Fatal("failed to select the submission for judging")
	}
	if err = room.JudgeButton(judge).JawsClick(elem, jaws.Click{}); err != nil {
		t.Fatalf("JudgeButton().JawsClick() error = %v", err)
	}
	if room.State() != StateReview {
		t.Fatalf("State() after judge click = %s, want %s", room.State(), StateReview)
	}
}

func TestStateRenderSnapshots(t *testing.T) {
	catalog := testCatalog(t)
	manager := NewManagerWithOptions(catalog, Options{MinPlayers: 2})
	host := testPlayer("Host")
	guest := testPlayer("Guest")
	room, err := manager.CreateRoom(host, catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err = manager.JoinRoom(room.Code(), guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}

	lobby := room.Lobby(host)
	if !lobby.Active {
		t.Fatalf("Lobby(host) = %#v, want active view", lobby)
	}
	if lobby.BlackCount < MinBlackCards || lobby.WhiteCount < lobby.RequiredWhite || lobby.MinimumTarget != 2 {
		t.Fatalf("Lobby(host) counts = %#v, want sufficient cards and minimum target 2", lobby)
	}
	if room.Playing(host).Active || room.Judging(host).Active || room.Review(host).Active {
		t.Fatal("non-lobby snapshots are active while the room is in the lobby")
	}

	if err = room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	forceRound(t, room, "b1")
	judge := room.JudgePlayer()
	contestant := host
	if contestant == judge {
		contestant = guest
	}
	hand := room.HandFor(contestant)
	if len(hand) == 0 || !room.ToggleCardSelection(contestant, hand[0]) {
		t.Fatal("failed to select a contestant card")
	}
	playing := room.Playing(contestant)
	if !playing.Active || !playing.CanSubmit || playing.Round.BlackCard != catalog.BlackCards["b1"] {
		t.Fatalf("Playing(contestant) = %#v, want active submit view for b1", playing)
	}
	if len(playing.Hand) != len(hand) || playing.Hand[0].Card != hand[0] || playing.Hand[0].SelectionOrder != 1 {
		t.Fatalf("Playing(contestant).Hand = %#v, want captured selected card first", playing.Hand)
	}
	judgeWaiting := room.Playing(judge)
	if !judgeWaiting.Active || judgeWaiting.CanSubmit || judgeWaiting.WaitingTitle != "Waiting for answers" || len(judgeWaiting.Hand) != 0 {
		t.Fatalf("Playing(judge) = %#v, want waiting judge view", judgeWaiting)
	}

	submission := &Submission{ID: "submission", Player: contestant, Cards: []*deck.WhiteCard{hand[0]}}
	room.mu.Lock()
	contestant.SelectedCards = nil
	room.state = StateJudging
	room.submissions = []*Submission{submission}
	judge.SelectedSubmission = submission
	room.mu.Unlock()

	if playing.Hand[0].SelectionOrder != 1 {
		t.Fatalf("captured selection order = %d after mutation, want 1", playing.Hand[0].SelectionOrder)
	}
	if room.Playing(contestant).Active {
		t.Fatal("Playing(contestant) remains active after state transition")
	}
	judging := room.Judging(judge)
	if !judging.Active || !judging.CanJudge || judging.Title != "Pick the Winner" || len(judging.Submissions) != 1 {
		t.Fatalf("Judging(judge) = %#v, want active judge view", judging)
	}
	if got := judging.Submissions[0]; got.Submission != submission || !got.Selected || got.Winning || !got.Enabled {
		t.Fatalf("Judging(judge).Submissions[0] = %#v, want selected enabled non-winner", got)
	}
	nonJudge := room.Judging(contestant)
	wantWaitingTitle := judge.Nickname + " is picking the winner"
	if nonJudge.Title != wantWaitingTitle || nonJudge.Submissions[0].Enabled {
		t.Fatalf("Judging(contestant) = %#v, want disabled waiting view", nonJudge)
	}

	room.mu.Lock()
	room.state = StateReview
	room.reviewWinner = contestant
	room.reviewSubmission = submission
	room.reviewDeadline = time.Now().Add(time.Hour)
	room.mu.Unlock()
	review := room.Review(judge)
	wantReviewTitle := contestant.Nickname + " won the round!"
	if !review.Active || review.Title != wantReviewTitle || review.Button == nil || len(review.Submissions) != 1 {
		t.Fatalf("Review(judge) = %#v, want active judge review", review)
	}
	if got := review.Submissions[0]; !got.Selected || !got.Winning || got.Enabled {
		t.Fatalf("Review(judge).Submissions[0] = %#v, want selected disabled winner", got)
	}

	room.mu.Lock()
	judge.SelectedSubmission = nil
	room.reviewSubmission = nil
	room.state = StatePlaying
	room.mu.Unlock()
	if got := review.Submissions[0]; !got.Selected || !got.Winning {
		t.Fatalf("captured review submission changed after mutation: %#v", got)
	}
	if room.Review(judge).Active {
		t.Fatal("Review(judge) remains active after state transition")
	}
}

func TestReviewSnapshotsDisplayAndControls(t *testing.T) {
	judge := testPlayer("Judge")
	winner := testPlayer("Winner")

	tests := []struct {
		name        string
		state       RoomState
		viewer      *Player
		gameWinner  bool
		remaining   time.Duration
		deadlineSet bool
		wantTitle   string
		wantStatus  string
		wantButton  string
	}{
		{name: "inactive", state: StatePlaying},
		{name: "round judge", state: StateReview, viewer: judge, remaining: 2500 * time.Millisecond, deadlineSet: true, wantTitle: "Winner won the round!", wantButton: "Next Round (3)"},
		{name: "round non-judge", state: StateReview, viewer: winner, remaining: 2500 * time.Millisecond, deadlineSet: true, wantTitle: "Winner won the round!", wantStatus: "Next round in 3 seconds."},
		{name: "round non-judge one second", state: StateReview, viewer: winner, remaining: time.Second, deadlineSet: true, wantTitle: "Winner won the round!", wantStatus: "Next round in 1 second."},
		{name: "round elapsed", state: StateReview, viewer: winner, deadlineSet: true, wantTitle: "Winner won the round!", wantStatus: "Advancing to the next round."},
		{name: "round judge without deadline", state: StateReview, viewer: judge, wantTitle: "Winner won the round!", wantButton: "Next Round"},
		{name: "game judge", state: StateReview, viewer: judge, gameWinner: true, remaining: 2500 * time.Millisecond, deadlineSet: true, wantTitle: "Winner won the game!", wantButton: "Back to Lobby (3)"},
		{name: "game non-judge", state: StateReview, viewer: winner, gameWinner: true, remaining: 2500 * time.Millisecond, deadlineSet: true, wantTitle: "Winner won the game!", wantStatus: "Returning to the lobby in 3 seconds."},
		{name: "game non-judge one second", state: StateReview, viewer: winner, gameWinner: true, remaining: time.Second, deadlineSet: true, wantTitle: "Winner won the game!", wantStatus: "Returning to the lobby in 1 second."},
		{name: "game non-judge elapsed", state: StateReview, viewer: winner, gameWinner: true, deadlineSet: true, wantTitle: "Winner won the game!", wantStatus: "Returning to the lobby."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var deadline time.Time
				if tt.deadlineSet {
					deadline = time.Now().Add(tt.remaining)
				}
				room := &Room{
					state:            tt.state,
					players:          []*Player{judge, winner},
					czarIndex:        0,
					reviewWinner:     winner,
					reviewGameWinner: tt.gameWinner,
					reviewDeadline:   deadline,
				}
				if tt.state != StateReview {
					room.reviewWinner = nil
				}

				got := room.Review(tt.viewer)
				if got.Title != tt.wantTitle {
					t.Fatalf("Title = %q, want %q", got.Title, tt.wantTitle)
				}
				if tt.wantStatus == "" {
					if got.Status != nil {
						t.Fatalf("Status = %#v, want nil", got.Status)
					}
				} else {
					if got.Status == nil {
						t.Fatal("Status = nil, want getter")
					}
					if text := got.Status.JawsGet(nil); text != tt.wantStatus {
						t.Fatalf("Status text = %q, want %q", text, tt.wantStatus)
					}
					tags, err := tag.TagExpand(got.Status)
					if err != nil {
						t.Fatalf("TagExpand(Status) error = %v", err)
					}
					if len(tags) != 1 || tags[0] != &room.reviewDeadline {
						t.Fatalf("Status tags = %#v, want [%p]", tags, &room.reviewDeadline)
					}
				}
				if tt.wantButton == "" {
					if got.Button != nil {
						t.Fatalf("Button = %#v, want nil", got.Button)
					}
					return
				}
				if got.Button == nil {
					t.Fatal("Button = nil, want action")
				}
				if content := string(got.Button.JawsGetHTML(nil)); content != tt.wantButton {
					t.Fatalf("Button content = %q, want %q", content, tt.wantButton)
				}
				if attrs := got.Button.JawsInitialHTMLAttr(nil); attrs != "" {
					t.Fatalf("Button attrs = %q, want none", attrs)
				}
				tags, err := tag.TagExpand(got.Button)
				if err != nil {
					t.Fatalf("TagExpand(Button) error = %v", err)
				}
				if len(tags) != 1 || tags[0] != &room.reviewDeadline {
					t.Fatalf("Button tags = %#v, want [%p]", tags, &room.reviewDeadline)
				}
			})
		})
	}
}

func TestReviewButtonRevalidatesStateAndJudge(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	judge := testPlayer("Judge")
	winner := testPlayer("Winner")
	newReviewRoom := func() *Room {
		return &Room{
			state:          StateReview,
			players:        []*Player{judge, winner},
			czarIndex:      0,
			reviewWinner:   winner,
			reviewDeadline: now.Add(time.Minute),
		}
	}
	elem := newJawsTestElement(t)

	t.Run("proceeds", func(t *testing.T) {
		room := newReviewRoom()
		room.reviewGameWinner = true
		button := room.Review(judge).Button
		if err := button.JawsClick(elem, jaws.Click{}); err != nil {
			t.Fatalf("JawsClick() error = %v", err)
		}
		if got := room.State(); got != StateLobby {
			t.Fatalf("State() = %s, want %s", got, StateLobby)
		}
	})

	t.Run("state changed", func(t *testing.T) {
		room := newReviewRoom()
		button := room.Review(judge).Button
		room.mu.Lock()
		room.state = StatePlaying
		room.mu.Unlock()
		if err := button.JawsClick(elem, jaws.Click{}); !errors.Is(err, ErrReviewNotReady) {
			t.Fatalf("JawsClick() error = %v, want %v", err, ErrReviewNotReady)
		}
	})

	t.Run("judge changed", func(t *testing.T) {
		room := newReviewRoom()
		button := room.Review(judge).Button
		room.mu.Lock()
		room.czarIndex = 1
		room.mu.Unlock()
		if err := button.JawsClick(elem, jaws.Click{}); !errors.Is(err, ErrNotJudge) {
			t.Fatalf("JawsClick() error = %v, want %v", err, ErrNotJudge)
		}
	})
}

func TestReviewConcurrentWithWinnerNicknameChanges(t *testing.T) {
	judge := testPlayer("Judge")
	winner := testPlayer("Winner")
	room := &Room{
		state:          StateReview,
		players:        []*Player{judge, winner},
		czarIndex:      0,
		reviewWinner:   winner,
		reviewDeadline: time.Now().Add(time.Hour),
	}
	judge.setRoom(room)
	winner.setRoom(room)

	start := make(chan struct{})
	errs := make(chan string, 1)
	report := func(message string) {
		select {
		case errs <- message:
		default:
		}
	}
	const iterations = 2000
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < iterations; i++ {
			if i%2 == 0 {
				room.setNickname(winner, "Alice")
			} else {
				room.setNickname(winner, "Casey")
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < iterations; i++ {
			title := room.Review(judge).Title
			switch title {
			case "Winner won the round!", "Alice won the round!", "Casey won the round!":
			default:
				report(fmt.Sprintf("Review().Title = %q", title))
				return
			}
		}
	}()
	close(start)
	wg.Wait()

	select {
	case message := <-errs:
		t.Fatal(message)
	default:
	}
}

func newJawsTestElement(t *testing.T) (result *jaws.Element) {
	t.Helper()
	jw, err := jaws.New()
	if err != nil {
		t.Fatalf("jaws.New() error = %v", err)
	}
	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		jw.Serve()
	}()
	request := jawstest.NewTestRequest(jw, nil)
	<-request.ReadyCh
	t.Cleanup(func() {
		request.Close()
		<-request.DoneCh
		jw.Close()
		<-serveDone
	})
	result = request.NewElement(nil)
	return
}

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

func TestRoomRenderAccessorsReadCurrentState(t *testing.T) {
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

	if black, white, required := room.BlackCount(), room.WhiteCount(), room.RequiredWhite(); black < MinBlackCards || white < required {
		t.Fatalf("selected cards = %d black / %d white, want at least %d / %d", black, white, MinBlackCards, required)
	}
	if minimum := room.MinTargetScore(); minimum != 2 {
		t.Fatalf("MinTargetScore() = %d, want 2", minimum)
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
	if !room.CanSubmit(contestant) || room.CurrentBlack() != catalog.BlackCards["b1"] {
		t.Fatal("current playing state is not directly visible through Room accessors")
	}
	if got := room.SelectionOrderFor(contestant, hand[0]); got != 1 {
		t.Fatalf("SelectionOrderFor() = %d, want 1", got)
	}
	if room.CanSubmit(judge) {
		t.Fatal("judge can submit")
	}

	submission := &Submission{ID: "submission", Player: contestant, Cards: []*deck.WhiteCard{hand[0]}}
	room.mu.Lock()
	contestant.SelectedCards = nil
	room.state = StateJudging
	room.submissions = []*Submission{submission}
	judge.SelectedSubmission = submission
	room.mu.Unlock()

	if got := room.SelectionOrderFor(contestant, hand[0]); got != 0 {
		t.Fatalf("SelectionOrderFor() after clearing selection = %d, want 0", got)
	}
	if !room.CanJudge(judge) || room.JudgeName() != judge.Nickname {
		t.Fatal("current judge is not directly visible through Room accessors")
	}
	if got := room.Submissions(); len(got) != 1 || got[0] != submission {
		t.Fatalf("Submissions() = %#v, want [%#v]", got, submission)
	}
	if !room.SubmissionSelected(judge, submission) || room.SubmissionSelected(contestant, submission) {
		t.Fatal("SubmissionSelected() does not reflect the current judge selection")
	}

	room.mu.Lock()
	room.state = StateReview
	room.reviewWinner = contestant
	room.reviewSubmission = submission
	room.reviewDeadline = time.Now().Add(time.Hour)
	room.mu.Unlock()
	if title := room.ReviewTitle(); title != contestant.Nickname+" won the round!" {
		t.Fatalf("ReviewTitle() = %q", title)
	}
	if room.ReviewButton(judge) == nil || room.ReviewStatus(judge) != nil {
		t.Fatal("judge review controls do not reflect the current review")
	}
	if !room.IsWinningSubmission(submission) {
		t.Fatal("IsWinningSubmission() = false for the review winner")
	}

	room.mu.Lock()
	judge.SelectedSubmission = nil
	room.reviewSubmission = nil
	room.state = StatePlaying
	room.mu.Unlock()
	if room.ReviewTitle() != "" || room.ReviewButton(judge) != nil || room.ReviewStatus(judge) != nil {
		t.Fatal("review accessors remain active after state transition")
	}
}

func TestReviewDisplayAndControls(t *testing.T) {
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

				title := room.ReviewTitle()
				status := room.ReviewStatus(tt.viewer)
				button := room.ReviewButton(tt.viewer)
				if title != tt.wantTitle {
					t.Fatalf("ReviewTitle() = %q, want %q", title, tt.wantTitle)
				}
				if tt.wantStatus == "" {
					if status != nil {
						t.Fatalf("ReviewStatus() = %#v, want nil", status)
					}
				} else {
					if status == nil {
						t.Fatal("ReviewStatus() = nil, want getter")
					}
					if text := status.JawsGet(nil); text != tt.wantStatus {
						t.Fatalf("Status text = %q, want %q", text, tt.wantStatus)
					}
					tags, err := tag.TagExpand(status)
					if err != nil {
						t.Fatalf("TagExpand(Status) error = %v", err)
					}
					if len(tags) != 1 || tags[0] != &room.reviewDeadline {
						t.Fatalf("Status tags = %#v, want [%p]", tags, &room.reviewDeadline)
					}
				}
				if tt.wantButton == "" {
					if button != nil {
						t.Fatalf("ReviewButton() = %#v, want nil", button)
					}
					return
				}
				if button == nil {
					t.Fatal("ReviewButton() = nil, want action")
				}
				if content := string(button.JawsGetHTML(nil)); content != tt.wantButton {
					t.Fatalf("Button content = %q, want %q", content, tt.wantButton)
				}
				if attrs := button.JawsInitialHTMLAttr(nil); attrs != "" {
					t.Fatalf("Button attrs = %q, want none", attrs)
				}
				tags, err := tag.TagExpand(button)
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
		button := room.ReviewButton(judge)
		if err := button.JawsClick(elem, jaws.Click{}); err != nil {
			t.Fatalf("JawsClick() error = %v", err)
		}
		if got := room.State(); got != StateLobby {
			t.Fatalf("State() = %s, want %s", got, StateLobby)
		}
	})

	t.Run("state changed", func(t *testing.T) {
		room := newReviewRoom()
		button := room.ReviewButton(judge)
		room.mu.Lock()
		room.state = StatePlaying
		room.mu.Unlock()
		if err := button.JawsClick(elem, jaws.Click{}); !errors.Is(err, ErrReviewNotReady) {
			t.Fatalf("JawsClick() error = %v, want %v", err, ErrReviewNotReady)
		}
	})

	t.Run("judge changed", func(t *testing.T) {
		room := newReviewRoom()
		button := room.ReviewButton(judge)
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
			title := room.ReviewTitle()
			switch title {
			case "Winner won the round!", "Alice won the round!", "Casey won the round!":
			default:
				report(fmt.Sprintf("ReviewTitle() = %q", title))
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

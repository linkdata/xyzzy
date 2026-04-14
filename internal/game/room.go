package game

import (
	"fmt"
	"html/template"
	mathrand "math/rand"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/xyzzy/internal/deck"
)

type RoomState string

const (
	StateLobby   RoomState = "lobby"
	StatePlaying RoomState = "playing"
	StateJudging RoomState = "judging"
	StateReview  RoomState = "results"
)

type Room struct {
	manager          *Manager
	code             string
	catalog          *deck.Catalog
	rand             *mathrand.Rand
	minPlayers       int
	debug            bool
	mu               sync.RWMutex
	host             *Player
	players          []*Player
	selectedDeckIDs  []string
	private          bool
	targetScore      int
	state            RoomState
	round            int
	czarIndex        int
	currentBlack     *deck.BlackCard
	submissionSeq    int
	lastWinnerName   string
	lastGameWinner   string
	lastGameScores   []FinalScore
	statusMessage    string
	blackDraw        []*deck.BlackCard
	blackDiscard     []*deck.BlackCard
	whiteDraw        []*deck.WhiteCard
	whiteDiscard     []*deck.WhiteCard
	submissions      []*Submission
	reviewDelay      time.Duration
	reviewTimer      *time.Timer
	reviewDeadline   time.Time
	reviewWinner     *Player
	reviewSubmission *Submission
	reviewGameWinner bool
	reviewToken      uint64
}

func (r *Room) ScoreTargetSlider(player *Player) bind.Binder[int] {
	return bind.New(&r.mu, &r.targetScore).
		SetLocked(func(bind bind.Binder[int], elem *jaws.Element, value int) (err error) {
			return r.setTargetScoreLocked(player, value)
		})
}

func (r *Room) PrivateToggle(player *Player) bind.Binder[bool] {
	return bind.New(&r.mu, &r.private).
		SetLocked(func(bind bind.Binder[bool], elem *jaws.Element, value bool) (err error) {
			if err = r.setPrivateLocked(player, value); err == nil {
				elem.Dirty(r.manager, r)
			}
			return
		})
}

func (r *Room) PrivateToggleAttrs(player *Player) template.HTMLAttr {
	if r.host == player && r.state == StateLobby {
		return ""
	}
	return `disabled`
}

func (r *Room) ScoreTargetAttrs(player *Player) template.HTMLAttr {
	if r.IsHost(player) && r.state == StateLobby {
		return ""
	}
	return `disabled`
}

func (r *Room) StartGameAttrs(player *Player) template.HTMLAttr {
	if !r.IsHost(player) {
		return `hidden`
	}
	if !r.CanStart(player) {
		return `disabled`
	}
	return ""
}

func (r *Room) SubmitCardsAttrs(player *Player) template.HTMLAttr {
	if !r.CanSubmit(player) || len(player.SelectedCards) != r.NeedPick() {
		return `disabled`
	}
	return ""
}

func (r *Room) SubmitCardsClick(player *Player) jaws.ClickHandler {
	return ui.Clickable("Play Selected Cards", func(elem *jaws.Element, name string) (err error) {
		selected := append([]*deck.WhiteCard(nil), player.SelectedCards...)
		if err = r.PlayCards(player, selected); err == nil {
			player.SelectedCards = nil
			elem.Dirty(player, r)
		}
		return
	})
}

func (r *Room) JudgeAttrs(player *Player) template.HTMLAttr {
	if !r.CanJudge(player) || player.SelectedSubmission == nil {
		return `disabled`
	}
	return ""
}

func (r *Room) JudgeClick(player *Player) jaws.ClickHandler {
	return ui.Clickable("Pick Winner", func(elem *jaws.Element, name string) (err error) {
		selected := player.SelectedSubmission
		if err = r.Judge(player, selected); err == nil {
			player.SelectedSubmission = nil
			elem.Dirty(player, r)
		}
		return nil
	})
}

func (r *Room) ProceedReviewAttrs(player *Player) template.HTMLAttr {
	if !r.CanProceed(player) {
		return `hidden`
	}
	return template.HTMLAttr(fmt.Sprintf(
		`class="btn btn-primary review-countdown-button" data-review-deadline="%d" data-review-label="%s"`,
		r.ReviewDeadlineUnixMilli(),
		r.ReviewButtonBase(),
	))
}

func (r *Room) ProceedReviewClick(player *Player) jaws.ClickHandler {
	return ui.Clickable("", func(elem *jaws.Element, name string) (err error) {
		if err = r.ProceedReview(player); err == nil {
			elem.Dirty(r)
		}
		return
	})
}

func (r *Room) StartGameClick(player *Player) jaws.ClickHandler {
	return ui.Clickable("Start Game", func(elem *jaws.Element, name string) (err error) {
		if err = r.Start(player); err == nil {
			player.SelectedCards = nil
			player.SelectedSubmission = nil
			elem.Dirty(player, r)
		}
		return
	})
}

func (r *Room) Code() string { return r.code }

func (r *Room) Locker() *sync.RWMutex { return &r.mu }

func (r *Room) TargetScorePtr() *int { return &r.targetScore }

func (r *Room) State() RoomState {
	r.mu.RLock()
	state := r.state
	r.mu.RUnlock()
	return state
}

func (r *Room) Host() *Player {
	r.mu.RLock()
	host := r.host
	r.mu.RUnlock()
	return host
}

func (r *Room) HostName() string {
	r.mu.RLock()
	host := r.host
	r.mu.RUnlock()
	if host == nil {
		return ""
	}
	return host.Nickname
}

func (r *Room) Players() []*Player {
	r.mu.RLock()
	players := append([]*Player(nil), r.players...)
	r.mu.RUnlock()
	return players
}

func (r *Room) PlayerCount() int {
	r.mu.RLock()
	count := len(r.players)
	r.mu.RUnlock()
	return count
}

func (r *Room) ScoreFor(player *Player) int {
	r.mu.RLock()
	score := 0
	if current := r.playerLocked(player); current != nil {
		score = current.Score
	}
	r.mu.RUnlock()
	return score
}

func (r *Room) SubmittedBy(player *Player) bool {
	r.mu.RLock()
	submitted := false
	if current := r.playerLocked(player); current != nil {
		submitted = len(current.Submitted) > 0
	}
	r.mu.RUnlock()
	return submitted
}

func (r *Room) HasPlayer(player *Player) bool {
	r.mu.RLock()
	ok := r.playerLocked(player) != nil
	r.mu.RUnlock()
	return ok
}

func (r *Room) IsHost(player *Player) bool {
	r.mu.RLock()
	ok := player != nil && r.host == player
	r.mu.RUnlock()
	return ok
}

func (r *Room) IsJudge(player *Player) bool {
	r.mu.RLock()
	ok := player != nil && r.state != StateLobby && r.judgeLocked() == player
	r.mu.RUnlock()
	return ok
}

func (r *Room) CanJoin(player *Player) bool {
	r.mu.RLock()
	canJoin := r.canJoinLocked(player) == nil
	r.mu.RUnlock()
	return canJoin
}

func (r *Room) CanStart(player *Player) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if player == nil || r.host != player || r.state != StateLobby || len(r.players) < r.minPlayers {
		return false
	}
	blackCount, whiteCount, err := r.catalog.UnionCounts(r.selectedDeckIDs)
	if err != nil {
		return false
	}
	return blackCount >= MinBlackCards && whiteCount >= MinWhiteCardsPerPlayer*len(r.players)
}

func (r *Room) CanSubmit(player *Player) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	current := r.playerLocked(player)
	return current != nil && r.state == StatePlaying && r.judgeLocked() != current && len(current.Submitted) == 0
}

func (r *Room) CanJudge(player *Player) bool {
	r.mu.RLock()
	ok := player != nil && r.state == StateJudging && r.judgeLocked() == player
	r.mu.RUnlock()
	return ok
}

func (r *Room) CanProceed(player *Player) bool {
	r.mu.RLock()
	ok := player != nil && r.state == StateReview && r.judgeLocked() == player
	r.mu.RUnlock()
	return ok
}

func (r *Room) SelectedDecks() []*deck.Deck {
	r.mu.RLock()
	decks := make([]*deck.Deck, 0, len(r.selectedDeckIDs))
	for _, id := range r.selectedDeckIDs {
		if d := r.catalog.DeckByID(id); d != nil {
			decks = append(decks, d)
		}
	}
	r.mu.RUnlock()
	return decks
}

func (r *Room) DeckEnabled(d *deck.Deck) bool {
	if d == nil {
		return false
	}
	r.mu.RLock()
	enabled := slices.Contains(r.selectedDeckIDs, d.ID)
	r.mu.RUnlock()
	return enabled
}

func (r *Room) BlackCount() int {
	r.mu.RLock()
	blackCount, _, _ := r.catalog.UnionCounts(r.selectedDeckIDs)
	r.mu.RUnlock()
	return blackCount
}

func (r *Room) WhiteCount() int {
	r.mu.RLock()
	_, whiteCount, _ := r.catalog.UnionCounts(r.selectedDeckIDs)
	r.mu.RUnlock()
	return whiteCount
}

func (r *Room) RequiredWhite() int {
	r.mu.RLock()
	required := MinWhiteCardsPerPlayer * max(len(r.players), 1)
	r.mu.RUnlock()
	return required
}

func (r *Room) TargetScore() int {
	r.mu.RLock()
	score := r.targetScore
	r.mu.RUnlock()
	return score
}

func (r *Room) IsPrivate() bool {
	r.mu.RLock()
	private := r.private
	r.mu.RUnlock()
	return private
}

func (r *Room) MinTargetScore() int {
	r.mu.RLock()
	minScore := r.minTargetScoreLocked()
	r.mu.RUnlock()
	return minScore
}

func (r *Room) CurrentBlack() *deck.BlackCard {
	r.mu.RLock()
	card := r.currentBlackLocked()
	r.mu.RUnlock()
	return card
}

func (r *Room) NeedPick() int {
	r.mu.RLock()
	pick := 0
	if black := r.currentBlackLocked(); black != nil {
		pick = black.Pick
	}
	r.mu.RUnlock()
	return pick
}

func (r *Room) NeedDraw() int {
	r.mu.RLock()
	draw := 0
	if black := r.currentBlackLocked(); black != nil {
		draw = black.Draw
	}
	r.mu.RUnlock()
	return draw
}

func (r *Room) HandFor(player *Player) []*deck.WhiteCard {
	r.mu.RLock()
	var cards []*deck.WhiteCard
	if current := r.playerLocked(player); current != nil {
		cards = append([]*deck.WhiteCard(nil), current.Hand...)
	}
	r.mu.RUnlock()
	return cards
}

func (r *Room) Submissions() []*Submission {
	r.mu.RLock()
	submissions := append([]*Submission(nil), r.submissions...)
	r.mu.RUnlock()
	return submissions
}

func (r *Room) SubmissionCards(submission *Submission) []*deck.WhiteCard {
	if submission == nil {
		return nil
	}
	r.mu.RLock()
	cards := append([]*deck.WhiteCard(nil), submission.Cards...)
	r.mu.RUnlock()
	return cards
}

func (r *Room) JudgePlayer() *Player {
	r.mu.RLock()
	judge := r.judgeLocked()
	r.mu.RUnlock()
	return judge
}

func (r *Room) JudgeName() string {
	r.mu.RLock()
	judge := r.judgeLocked()
	r.mu.RUnlock()
	if judge == nil {
		return ""
	}
	return judge.Nickname
}

func (r *Room) StatusMessage() string {
	r.mu.RLock()
	message := r.statusMessage
	r.mu.RUnlock()
	return message
}

func (r *Room) LastWinnerName() string {
	r.mu.RLock()
	name := r.lastWinnerName
	r.mu.RUnlock()
	return name
}

func (r *Room) LastGameWinner() string {
	r.mu.RLock()
	name := r.lastGameWinner
	r.mu.RUnlock()
	return name
}

func (r *Room) LastGameScores() []FinalScore {
	r.mu.RLock()
	scores := append([]FinalScore(nil), r.lastGameScores...)
	r.mu.RUnlock()
	return scores
}

func (r *Room) ReviewTitle() string {
	r.mu.RLock()
	title := ""
	if r.state == StateReview && r.reviewWinner != nil {
		if r.reviewGameWinner {
			title = fmt.Sprintf("%s won the game!", r.reviewWinner.Nickname)
		} else {
			title = fmt.Sprintf("%s won the round!", r.reviewWinner.Nickname)
		}
	}
	r.mu.RUnlock()
	return title
}

func (r *Room) ReviewCountdown() int {
	r.mu.RLock()
	deadline := r.reviewDeadline
	state := r.state
	r.mu.RUnlock()
	if state != StateReview || deadline.IsZero() {
		return 0
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 0
	}
	return int((remaining + time.Second - time.Nanosecond) / time.Second)
}

func (r *Room) ReviewDeadlineUnixMilli() int64 {
	r.mu.RLock()
	deadline := r.reviewDeadline
	r.mu.RUnlock()
	if deadline.IsZero() {
		return 0
	}
	return deadline.UnixMilli()
}

func (r *Room) ReviewButtonBase() string {
	r.mu.RLock()
	base := r.reviewButtonBaseLocked()
	r.mu.RUnlock()
	return base
}

func (r *Room) ReviewProceedLabel() string {
	base := r.ReviewButtonBase()
	countdown := r.ReviewCountdown()
	if countdown <= 0 {
		return base
	}
	return fmt.Sprintf("%s (%d)", base, countdown)
}

func (r *Room) ReviewWaitingText() string {
	base := r.ReviewButtonBase()
	countdown := r.ReviewCountdown()
	switch {
	case base == "":
		return ""
	case countdown <= 0 && base == "Back to Lobby":
		return "Returning to the lobby."
	case countdown <= 0:
		return "Advancing to the next round."
	case base == "Back to Lobby":
		return fmt.Sprintf("Returning to the lobby in %d seconds.", countdown)
	default:
		return fmt.Sprintf("Next round in %d seconds.", countdown)
	}
}

func (r *Room) IsRoundWinner(player *Player) bool {
	r.mu.RLock()
	ok := r.state == StateReview && player != nil && r.reviewWinner == player
	r.mu.RUnlock()
	return ok
}

func (r *Room) IsWinningSubmission(submission *Submission) bool {
	r.mu.RLock()
	ok := r.state == StateReview && submission != nil && r.reviewSubmission == submission
	r.mu.RUnlock()
	return ok
}

func (r *Room) SetPrivate(player *Player, private bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.setPrivateLocked(player, private)
}

func (r *Room) SetNickname(player *Player, nickname string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.playerLocked(player)
	if current == nil {
		return
	}
	current.Nickname = NormalizeNickname(nickname)
	current.NicknameInput = current.Nickname
	current.Nickname = r.uniqueNicknameLocked(current)
	current.NicknameInput = current.Nickname
}

func (r *Room) SetTargetScore(player *Player, score int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.setTargetScoreLocked(player, score)
}

func (r *Room) SetDeckEnabled(player *Player, deck *deck.Deck, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.host != player {
		return ErrOnlyHostCanEdit
	}
	if r.state != StateLobby {
		return ErrDecksLocked
	}
	if deck == nil || r.catalog.DeckByID(deck.ID) == nil {
		return ErrUnknownDeck
	}
	selected := make(map[string]bool, len(r.selectedDeckIDs))
	for _, id := range r.selectedDeckIDs {
		selected[id] = true
	}
	if enabled {
		selected[deck.ID] = true
	} else {
		delete(selected, deck.ID)
	}
	r.selectedDeckIDs = sortedSelected(selected)
	return nil
}

func (r *Room) Start(player *Player) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.host != player {
		return ErrOnlyHostCanStart
	}
	if len(r.players) < r.minPlayers {
		return fmt.Errorf("need at least %d players to start", r.minPlayers)
	}
	blackCount, whiteCount, err := r.catalog.UnionCounts(r.selectedDeckIDs)
	if err != nil {
		return err
	}
	if blackCount < MinBlackCards {
		return ErrNotEnoughBlackCards
	}
	if whiteCount < MinWhiteCardsPerPlayer*len(r.players) {
		return ErrNotEnoughWhiteCards
	}
	blackCards, whiteCards, err := r.catalog.UnionCards(r.selectedDeckIDs)
	if err != nil {
		return err
	}
	r.blackDraw = append([]*deck.BlackCard(nil), blackCards...)
	r.whiteDraw = append([]*deck.WhiteCard(nil), whiteCards...)
	r.rand.Shuffle(len(r.blackDraw), func(i, j int) { r.blackDraw[i], r.blackDraw[j] = r.blackDraw[j], r.blackDraw[i] })
	r.rand.Shuffle(len(r.whiteDraw), func(i, j int) { r.whiteDraw[i], r.whiteDraw[j] = r.whiteDraw[j], r.whiteDraw[i] })
	r.prepareOpeningBlackLocked(blackCards)
	r.blackDiscard = nil
	r.whiteDiscard = nil
	r.submissions = nil
	r.clearReviewLocked()
	r.currentBlack = nil
	r.submissionSeq = 0
	r.lastWinnerName = ""
	r.lastGameWinner = ""
	r.lastGameScores = nil
	r.statusMessage = ""
	r.round = 0
	r.czarIndex = -1
	for _, current := range r.players {
		current.Score = 0
		current.Hand = nil
		current.Submitted = nil
	}
	r.advanceRoundLocked()
	return nil
}

func (r *Room) PlayCards(player *Player, cards []*deck.WhiteCard) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.playerLocked(player)
	if current == nil {
		return ErrOnlyPlayersCanPlay
	}
	if r.state != StatePlaying {
		return ErrNotYourTurn
	}
	if r.judgeLocked() == current {
		return ErrJudgeCannotPlay
	}
	if len(current.Submitted) > 0 {
		return ErrAlreadySubmitted
	}
	cards = normalizeWhiteCards(cards)
	if len(cards) != r.currentBlackLocked().Pick {
		return ErrNeedExactCards
	}
	handSet := make(map[*deck.WhiteCard]struct{}, len(current.Hand))
	for _, card := range current.Hand {
		handSet[card] = struct{}{}
	}
	for _, card := range cards {
		if _, ok := handSet[card]; !ok {
			return ErrCardNotInHand
		}
	}
	remaining := make([]*deck.WhiteCard, 0, len(current.Hand)-len(cards))
	selected := make(map[*deck.WhiteCard]struct{}, len(cards))
	for _, card := range cards {
		selected[card] = struct{}{}
	}
	for _, card := range current.Hand {
		if _, ok := selected[card]; ok {
			continue
		}
		remaining = append(remaining, card)
	}
	current.Hand = remaining
	current.Submitted = append([]*deck.WhiteCard(nil), cards...)
	r.submissionSeq++
	r.submissions = append(r.submissions, &Submission{
		ID:     submissionID(r.round, r.submissionSeq),
		Player: current,
		Cards:  append([]*deck.WhiteCard(nil), cards...),
	})
	if len(r.submissions) == len(r.players)-1 {
		r.rand.Shuffle(len(r.submissions), func(i, j int) {
			r.submissions[i], r.submissions[j] = r.submissions[j], r.submissions[i]
		})
		r.state = StateJudging
		if judge := r.judgeLocked(); judge != nil {
			r.statusMessage = fmt.Sprintf("%s is judging the round.", judge.Nickname)
		}
	}
	return nil
}

func (r *Room) Judge(player *Player, submission *Submission) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != StateJudging {
		return ErrNotYourTurn
	}
	if r.judgeLocked() != player {
		return ErrNotJudge
	}
	var winner *Player
	for _, candidate := range r.submissions {
		if candidate == submission {
			winner = candidate.Player
			break
		}
	}
	if winner == nil {
		return ErrSubmissionNotFound
	}
	winner.Score++
	r.lastWinnerName = winner.Nickname
	gameWinner := winner.Score >= r.targetScore
	if gameWinner {
		r.captureLastGameLocked(winner)
	}
	r.statusMessage = fmt.Sprintf("%s won the round.", winner.Nickname)
	r.beginReviewLocked(winner, submission, gameWinner)
	return nil
}

func (r *Room) ProceedReview(player *Player) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != StateReview {
		return ErrReviewNotReady
	}
	if r.judgeLocked() != player {
		return ErrNotJudge
	}
	r.finishReviewLocked()
	return nil
}

func (r *Room) join(player *Player) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.playerLocked(player) != nil {
		return nil
	}
	if err := r.canJoinLocked(player); err != nil {
		return err
	}
	r.seatLocked(player)
	r.players = append(r.players, player)
	if r.host == nil {
		r.host = player
	}
	r.dealJoinedPlayerLocked(player)
	r.statusMessage = fmt.Sprintf("%s joined the room.", player.Nickname)
	return nil
}

func (r *Room) leave(player *Player) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.playerLocked(player)
	if current == nil {
		return len(r.players) == 0
	}
	wasJudge := r.judgeLocked() == current
	idx := slices.Index(r.players, current)
	if idx < 0 {
		return len(r.players) == 0
	}
	if idx < r.czarIndex {
		r.czarIndex--
	}
	r.whiteDiscard = append(r.whiteDiscard, current.Hand...)
	r.whiteDiscard = append(r.whiteDiscard, current.Submitted...)
	current.Room = nil
	current.Score = 0
	current.Hand = nil
	current.Submitted = nil
	current.SelectedCards = nil
	current.SelectedSubmission = nil
	r.players = append(r.players[:idx], r.players[idx+1:]...)
	r.submissions = slices.DeleteFunc(r.submissions, func(sub *Submission) bool { return sub.Player == current })
	if r.host == current {
		if len(r.players) > 0 {
			r.host = r.players[0]
		} else {
			r.host = nil
		}
	}
	if len(r.players) == 0 {
		return true
	}
	if r.state != StateLobby {
		switch {
		case len(r.players) < r.minPlayers:
			r.resetToLobbyLocked("Not enough players to continue. Room reset to the lobby.")
		case wasJudge:
			r.resetToLobbyLocked("The judge left. Room reset to the lobby.")
		case len(r.submissions) == len(r.players)-1 && r.state == StatePlaying:
			r.rand.Shuffle(len(r.submissions), func(i, j int) { r.submissions[i], r.submissions[j] = r.submissions[j], r.submissions[i] })
			r.state = StateJudging
			if judge := r.judgeLocked(); judge != nil {
				r.statusMessage = fmt.Sprintf("%s is judging the round.", judge.Nickname)
			}
		default:
			r.statusMessage = fmt.Sprintf("%s left the room.", current.Nickname)
		}
	} else {
		r.statusMessage = fmt.Sprintf("%s left the room.", current.Nickname)
	}
	return false
}

func (r *Room) expiredPlayers() []*Player {
	r.mu.RLock()
	expired := make([]*Player, 0)
	for _, player := range r.players {
		if player == nil || player.Session == nil || player.Session.Cookie().MaxAge < 0 {
			expired = append(expired, player)
		}
	}
	r.mu.RUnlock()
	return expired
}

func (r *Room) seatLocked(player *Player) {
	player.Nickname = r.uniqueNicknameLocked(player)
	player.NicknameInput = player.Nickname
	player.Room = r
	player.Score = 0
	player.Hand = nil
	player.Submitted = nil
	player.SelectedCards = nil
	player.SelectedSubmission = nil
}

func (r *Room) canJoinLocked(player *Player) error {
	if player == nil {
		return ErrRoomNotFound
	}
	if player.Room != nil {
		return ErrAlreadyInRoom
	}
	if len(r.players) >= MaxPlayers {
		return ErrRoomFull
	}
	if r.state == StateLobby {
		return nil
	}
	_, whiteCount, err := r.catalog.UnionCounts(r.selectedDeckIDs)
	if err != nil {
		return err
	}
	if whiteCount < MinWhiteCardsPerPlayer*(len(r.players)+1) {
		return ErrNotEnoughWhiteCards
	}
	return nil
}

func (r *Room) dealJoinedPlayerLocked(player *Player) {
	if player == nil || r.state == StateLobby {
		return
	}
	for len(player.Hand) < HandSize {
		player.Hand = append(player.Hand, r.drawWhiteLocked())
	}
	if r.state != StatePlaying {
		return
	}
	black := r.currentBlackLocked()
	if black == nil {
		return
	}
	for i := 0; i < black.Draw; i++ {
		player.Hand = append(player.Hand, r.drawWhiteLocked())
	}
}

func (r *Room) uniqueNicknameLocked(player *Player) string {
	base := NormalizeNickname(player.NicknameInput)
	if base == "Player" && strings.TrimSpace(player.Nickname) != "" {
		base = NormalizeNickname(player.Nickname)
	}
	candidate := base
	for suffix := 2; ; suffix++ {
		if !r.nicknameTakenLocked(candidate, player) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, suffix)
	}
}

func (r *Room) nicknameTakenLocked(candidate string, exclude *Player) bool {
	for _, player := range r.players {
		if player == exclude {
			continue
		}
		if strings.EqualFold(player.Nickname, candidate) {
			return true
		}
	}
	return false
}

func (r *Room) resetToLobbyLocked(message string) {
	r.clearReviewLocked()
	r.state = StateLobby
	r.round = 0
	r.czarIndex = -1
	r.currentBlack = nil
	r.submissionSeq = 0
	r.blackDraw = nil
	r.blackDiscard = nil
	r.whiteDraw = nil
	r.whiteDiscard = nil
	r.submissions = nil
	r.statusMessage = message
	for _, player := range r.players {
		player.Score = 0
		player.Hand = nil
		player.Submitted = nil
		player.SelectedCards = nil
		player.SelectedSubmission = nil
	}
}

func (r *Room) captureLastGameLocked(winner *Player) {
	r.lastGameWinner = ""
	r.lastGameScores = make([]FinalScore, 0, len(r.players))
	for _, player := range r.players {
		score := FinalScore{
			Player:   player,
			Nickname: player.Nickname,
			Score:    player.Score,
			IsWinner: player == winner,
		}
		if score.IsWinner {
			r.lastGameWinner = player.Nickname
		}
		r.lastGameScores = append(r.lastGameScores, score)
	}
	slices.SortStableFunc(r.lastGameScores, func(a, b FinalScore) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		return strings.Compare(a.Nickname, b.Nickname)
	})
}

func (r *Room) advanceRoundLocked() {
	r.clearReviewLocked()
	if r.currentBlack != nil {
		r.blackDiscard = append(r.blackDiscard, r.currentBlack)
	}
	for _, submission := range r.submissions {
		r.whiteDiscard = append(r.whiteDiscard, submission.Cards...)
	}
	for _, player := range r.players {
		player.Submitted = nil
	}
	r.submissions = nil
	r.submissionSeq = 0
	if len(r.players) == 0 {
		r.resetToLobbyLocked("Room is empty.")
		return
	}
	r.czarIndex++
	if r.czarIndex >= len(r.players) {
		r.czarIndex = 0
	}
	for _, player := range r.players {
		for len(player.Hand) < HandSize {
			player.Hand = append(player.Hand, r.drawWhiteLocked())
		}
	}
	r.currentBlack = r.drawBlackLocked()
	black := r.currentBlackLocked()
	judge := r.judgeLocked()
	for _, player := range r.players {
		if player == judge {
			continue
		}
		for i := 0; black != nil && i < black.Draw; i++ {
			player.Hand = append(player.Hand, r.drawWhiteLocked())
		}
	}
	r.round++
	r.state = StatePlaying
	if judge != nil {
		r.statusMessage = fmt.Sprintf("%s is the judge for round %d.", judge.Nickname, r.round)
	}
}

func (r *Room) beginReviewLocked(winner *Player, submission *Submission, gameWinner bool) {
	r.clearReviewLocked()
	r.state = StateReview
	r.reviewWinner = winner
	r.reviewSubmission = submission
	r.reviewGameWinner = gameWinner
	delay := r.reviewDelay
	if delay <= 0 {
		delay = ReviewDelay
	}
	r.reviewDeadline = time.Now().Add(delay)
	token := r.reviewToken
	r.reviewTimer = time.AfterFunc(delay, func() {
		r.autoProceedReview(token)
	})
}

func (r *Room) finishReviewLocked() {
	winnerName := ""
	if r.reviewWinner != nil {
		winnerName = r.reviewWinner.Nickname
	}
	if r.reviewGameWinner {
		r.resetToLobbyLocked(fmt.Sprintf("%s won the game. Room reset to the lobby.", winnerName))
		return
	}
	r.advanceRoundLocked()
}

func (r *Room) autoProceedReview(token uint64) {
	r.mu.Lock()
	if r.state != StateReview || r.reviewToken != token {
		r.mu.Unlock()
		return
	}
	r.finishReviewLocked()
	manager := r.manager
	r.mu.Unlock()
	if manager != nil {
		manager.notify(r)
	}
}

func (r *Room) clearReviewLocked() {
	if r.reviewTimer != nil {
		r.reviewTimer.Stop()
		r.reviewTimer = nil
	}
	r.reviewToken++
	r.reviewDeadline = time.Time{}
	r.reviewWinner = nil
	r.reviewSubmission = nil
	r.reviewGameWinner = false
}

func (r *Room) reviewButtonBaseLocked() string {
	if r.state != StateReview {
		return ""
	}
	if r.reviewGameWinner {
		return "Back to Lobby"
	}
	return "Next Round"
}

func (r *Room) drawWhiteLocked() *deck.WhiteCard {
	if len(r.whiteDraw) == 0 {
		r.whiteDraw = append(r.whiteDraw, r.whiteDiscard...)
		r.whiteDiscard = nil
		r.rand.Shuffle(len(r.whiteDraw), func(i, j int) { r.whiteDraw[i], r.whiteDraw[j] = r.whiteDraw[j], r.whiteDraw[i] })
	}
	card := r.whiteDraw[len(r.whiteDraw)-1]
	r.whiteDraw = r.whiteDraw[:len(r.whiteDraw)-1]
	return card
}

func (r *Room) drawBlackLocked() *deck.BlackCard {
	if len(r.blackDraw) == 0 {
		r.blackDraw = append(r.blackDraw, r.blackDiscard...)
		r.blackDiscard = nil
		r.rand.Shuffle(len(r.blackDraw), func(i, j int) { r.blackDraw[i], r.blackDraw[j] = r.blackDraw[j], r.blackDraw[i] })
	}
	card := r.blackDraw[len(r.blackDraw)-1]
	r.blackDraw = r.blackDraw[:len(r.blackDraw)-1]
	return card
}

func (r *Room) judgeLocked() *Player {
	if len(r.players) == 0 || r.czarIndex < 0 || r.czarIndex >= len(r.players) {
		return nil
	}
	return r.players[r.czarIndex]
}

func (r *Room) currentBlackLocked() *deck.BlackCard {
	return r.currentBlack
}

func (r *Room) playerLocked(player *Player) *Player {
	for _, current := range r.players {
		if current == player {
			return current
		}
	}
	return nil
}

func (r *Room) SelectedDeckIDsForWhiteCard(card *deck.WhiteCard) []string {
	if r == nil || card == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.selectedDeckIDsForWhiteCardLocked(card)
}

func (r *Room) SelectedDeckIDsForBlackCard(card *deck.BlackCard) []string {
	if r == nil || card == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.selectedDeckIDsForBlackCardLocked(card)
}

func (r *Room) FirstSelectedDeckNameForWhiteCard(card *deck.WhiteCard) string {
	if r == nil || card == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.firstSelectedDeckNameForWhiteCardLocked(card)
}

func (r *Room) FirstSelectedDeckNameForBlackCard(card *deck.BlackCard) string {
	if r == nil || card == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.firstSelectedDeckNameForBlackCardLocked(card)
}

func (r *Room) selectedDeckIDsForWhiteCardLocked(card *deck.WhiteCard) []string {
	deckIDs := make([]string, 0, len(r.selectedDeckIDs))
	for _, deckID := range r.selectedDeckIDs {
		d := r.catalog.DeckByID(deckID)
		if d == nil {
			continue
		}
		if slices.Contains(d.WhiteCards, card) {
			deckIDs = append(deckIDs, deckID)
		}
	}
	return deckIDs
}

func (r *Room) selectedDeckIDsForBlackCardLocked(card *deck.BlackCard) []string {
	deckIDs := make([]string, 0, len(r.selectedDeckIDs))
	for _, deckID := range r.selectedDeckIDs {
		d := r.catalog.DeckByID(deckID)
		if d == nil {
			continue
		}
		if slices.Contains(d.BlackCards, card) {
			deckIDs = append(deckIDs, deckID)
		}
	}
	return deckIDs
}

func (r *Room) firstSelectedDeckNameForWhiteCardLocked(card *deck.WhiteCard) string {
	for _, deckID := range r.selectedDeckIDs {
		d := r.catalog.DeckByID(deckID)
		if d == nil {
			continue
		}
		if slices.Contains(d.WhiteCards, card) {
			return d.Name
		}
	}
	return ""
}

func (r *Room) firstSelectedDeckNameForBlackCardLocked(card *deck.BlackCard) string {
	for _, deckID := range r.selectedDeckIDs {
		d := r.catalog.DeckByID(deckID)
		if d == nil {
			continue
		}
		if slices.Contains(d.BlackCards, card) {
			return d.Name
		}
	}
	return ""
}

func (r *Room) setTargetScoreLocked(player *Player, score int) error {
	if score < r.minTargetScoreLocked() {
		score = r.minTargetScoreLocked()
	} else if score > 10 {
		score = 10
	}
	if r.host != player {
		return ErrOnlyHostCanEdit
	}
	if r.state != StateLobby {
		return ErrGameInProgress
	}
	r.targetScore = score
	return nil
}

func (r *Room) setPrivateLocked(player *Player, private bool) error {
	if r.host != player {
		return ErrOnlyHostCanEdit
	}
	if r.state != StateLobby {
		return ErrGameInProgress
	}
	r.private = private
	return nil
}

func (r *Room) minTargetScoreLocked() int {
	if r.debug {
		return 1
	}
	return 2
}

func (r *Room) prepareOpeningBlackLocked(cards []*deck.BlackCard) {
	if !r.debug || len(r.blackDraw) == 0 {
		return
	}
	var best *deck.BlackCard
	bestPick := -1
	bestDraw := -1
	for _, card := range cards {
		if card == nil {
			continue
		}
		if card.Pick > bestPick || (card.Pick == bestPick && card.Draw > bestDraw) || (card.Pick == bestPick && card.Draw == bestDraw && (best == nil || card.ID < best.ID)) {
			best = card
			bestPick = card.Pick
			bestDraw = card.Draw
		}
	}
	if best == nil {
		return
	}
	for i, card := range r.blackDraw {
		if card == best {
			last := len(r.blackDraw) - 1
			r.blackDraw[i], r.blackDraw[last] = r.blackDraw[last], r.blackDraw[i]
			return
		}
	}
}

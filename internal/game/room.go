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
	selectedDecks    []*deck.Deck
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
			err = r.setTargetScoreLocked(player, value)
			return
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

func (r *Room) PrivateToggleAttrs(player *Player) (result template.HTMLAttr) {
	if !r.IsHost(player) || r.state != StateLobby {
		result = `disabled`
	}
	return
}

func (r *Room) ScoreTargetAttrs(player *Player) (result template.HTMLAttr) {
	if !r.IsHost(player) || r.state != StateLobby {
		result = `disabled`
	}
	return
}

func (r *Room) StartGameAttrs(player *Player) (result template.HTMLAttr) {
	if !r.IsHost(player) {
		result = `hidden`
		return
	}
	if !r.CanStart(player) {
		result = `disabled`
	}
	return
}

func (r *Room) SubmitCardsAttrs(player *Player) (result template.HTMLAttr) {
	if !r.CanSubmit(player) || len(player.SelectedCards) != r.NeedPick() {
		result = `disabled`
	}
	return
}

func (r *Room) SubmitCardsClick(player *Player) jaws.ClickHandler {
	return ui.New("Play Selected Cards").Clicked(func(obj ui.Object, elem *jaws.Element, click jaws.Click) (err error) {
		selected := append([]*deck.WhiteCard(nil), player.SelectedCards...)
		if err = r.PlayCards(player, selected); err == nil {
			player.SelectedCards = nil
			elem.Dirty(player, r)
		}
		return
	})
}

func (r *Room) JudgeAttrs(player *Player) (result template.HTMLAttr) {
	if !r.CanJudge(player) || player.SelectedSubmission == nil {
		result = `disabled`
	}
	return
}

func (r *Room) JudgeClick(player *Player) jaws.ClickHandler {
	return ui.New("Pick Winner").Clicked(func(obj ui.Object, elem *jaws.Element, click jaws.Click) (err error) {
		selected := player.SelectedSubmission
		if err = r.Judge(player, selected); err == nil {
			player.SelectedSubmission = nil
			elem.Dirty(player, r)
		}
		return

	})
}

func (r *Room) ProceedReviewAttrs(player *Player) (result template.HTMLAttr) {
	if !r.CanProceed(player) {
		result = `hidden`
		return
	}
	result = template.HTMLAttr(fmt.Sprintf(
		`class="btn btn-primary review-countdown-button" data-review-deadline="%d" data-review-label="%s"`,
		r.ReviewDeadlineUnixMilli(),
		r.ReviewButtonBase(),
	))
	return
}

func (r *Room) ProceedReviewClick(player *Player) jaws.ClickHandler {
	return ui.New("").Clicked(func(obj ui.Object, elem *jaws.Element, click jaws.Click) (err error) {
		if err = r.ProceedReview(player); err == nil {
			elem.Dirty(r)
		}
		return
	})
}

func (r *Room) StartGameClick(player *Player) jaws.ClickHandler {
	return ui.New("Start Game").Clicked(func(obj ui.Object, elem *jaws.Element, click jaws.Click) (err error) {
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

func (r *Room) State() (result RoomState) {
	r.mu.RLock()
	result = r.state
	r.mu.RUnlock()
	return
}

func (r *Room) Host() (result *Player) {
	r.mu.RLock()
	result = r.host
	r.mu.RUnlock()
	return
}

func (r *Room) HostName() (result string) {
	r.mu.RLock()
	host := r.host
	r.mu.RUnlock()
	if host != nil {
		result = host.Nickname
	}
	return
}

func (r *Room) Players() (result []*Player) {
	r.mu.RLock()
	result = append([]*Player(nil), r.players...)
	r.mu.RUnlock()
	return
}

func (r *Room) PlayerCount() (result int) {
	r.mu.RLock()
	result = len(r.players)
	r.mu.RUnlock()
	return
}

func (r *Room) ScoreFor(player *Player) (result int) {
	r.mu.RLock()
	if current := r.playerLocked(player); current != nil {
		result = current.Score
	}
	r.mu.RUnlock()
	return
}

func (r *Room) SubmittedBy(player *Player) (result bool) {
	r.mu.RLock()
	if current := r.playerLocked(player); current != nil {
		result = len(current.Submitted) > 0
	}
	r.mu.RUnlock()
	return
}

func (r *Room) HasPlayer(player *Player) (result bool) {
	r.mu.RLock()
	result = r.playerLocked(player) != nil
	r.mu.RUnlock()
	return
}

func (r *Room) IsHost(player *Player) (result bool) {
	r.mu.RLock()
	result = player != nil && r.host == player
	r.mu.RUnlock()
	return
}

func (r *Room) IsJudge(player *Player) (result bool) {
	r.mu.RLock()
	result = player != nil && r.state != StateLobby && r.judgeLocked() == player
	r.mu.RUnlock()
	return
}

func (r *Room) CanJoin(player *Player) (result bool) {
	r.mu.RLock()
	result = r.canJoinLocked(player) == nil
	r.mu.RUnlock()
	return
}

func (r *Room) CanStart(player *Player) (result bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if player == nil || r.host != player || r.state != StateLobby || len(r.players) < r.minPlayers {
		return
	}
	blackCount, whiteCount := r.catalog.UnionCounts(r.selectedDecks)
	result = blackCount >= MinBlackCards && whiteCount >= MinWhiteCardsPerPlayer*len(r.players)
	return
}

func (r *Room) CanSubmit(player *Player) (result bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	current := r.playerLocked(player)
	result = current != nil && r.state == StatePlaying && r.judgeLocked() != current && len(current.Submitted) == 0
	return
}

func (r *Room) CanJudge(player *Player) (result bool) {
	r.mu.RLock()
	result = player != nil && r.state == StateJudging && r.judgeLocked() == player
	r.mu.RUnlock()
	return
}

func (r *Room) CanProceed(player *Player) (result bool) {
	r.mu.RLock()
	result = player != nil && r.state == StateReview && r.judgeLocked() == player
	r.mu.RUnlock()
	return
}

func (r *Room) SelectedDecks() (result []*deck.Deck) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result = make([]*deck.Deck, 0, len(r.selectedDecks))
	result = append(result, r.selectedDecks...)
	return
}

func (r *Room) DeckEnabled(d *deck.Deck) (result bool) {
	if d != nil {
		r.mu.RLock()
		result = slices.Contains(r.selectedDecks, d)
		r.mu.RUnlock()
	}
	return
}

func (r *Room) BlackCount() (result int) {
	r.mu.RLock()
	result, _ = r.catalog.UnionCounts(r.selectedDecks)
	r.mu.RUnlock()
	return
}

func (r *Room) WhiteCount() (result int) {
	r.mu.RLock()
	_, result = r.catalog.UnionCounts(r.selectedDecks)
	r.mu.RUnlock()
	return
}

func (r *Room) RequiredWhite() (result int) {
	r.mu.RLock()
	result = MinWhiteCardsPerPlayer * max(len(r.players), 1)
	r.mu.RUnlock()
	return
}

func (r *Room) TargetScore() (result int) {
	r.mu.RLock()
	result = r.targetScore
	r.mu.RUnlock()
	return
}

func (r *Room) IsPrivate() (result bool) {
	r.mu.RLock()
	result = r.private
	r.mu.RUnlock()
	return
}

func (r *Room) MinTargetScore() (result int) {
	r.mu.RLock()
	result = r.minTargetScoreLocked()
	r.mu.RUnlock()
	return
}

func (r *Room) CurrentBlack() (result *deck.BlackCard) {
	r.mu.RLock()
	result = r.currentBlackLocked()
	r.mu.RUnlock()
	return
}

func (r *Room) NeedPick() (result int) {
	r.mu.RLock()
	if black := r.currentBlackLocked(); black != nil {
		result = black.Pick
	}
	r.mu.RUnlock()
	return
}

func (r *Room) NeedDraw() (result int) {
	r.mu.RLock()
	if black := r.currentBlackLocked(); black != nil {
		result = black.Draw
	}
	r.mu.RUnlock()
	return
}

func (r *Room) HandFor(player *Player) (cards []*deck.WhiteCard) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if current := r.playerLocked(player); current != nil {
		cards = append(cards, current.Hand...)
	}
	return
}

func (r *Room) Submissions() (submissions []*Submission) {
	r.mu.RLock()
	submissions = append(submissions, r.submissions...)
	r.mu.RUnlock()
	return
}

func (r *Room) SubmissionCards(submission *Submission) (cards []*deck.WhiteCard) {
	if submission != nil {
		r.mu.RLock()
		cards = append(cards, submission.Cards...)
		r.mu.RUnlock()
	}
	return
}

func (r *Room) JudgePlayer() (result *Player) {
	r.mu.RLock()
	result = r.judgeLocked()
	r.mu.RUnlock()
	return
}

func (r *Room) JudgeName() (result string) {
	r.mu.RLock()
	judge := r.judgeLocked()
	r.mu.RUnlock()
	if judge != nil {
		result = judge.Nickname
	}
	return
}

func (r *Room) LastWinnerName() (result string) {
	r.mu.RLock()
	result = r.lastWinnerName
	r.mu.RUnlock()
	return
}

func (r *Room) LastGameWinner() (result string) {
	r.mu.RLock()
	result = r.lastGameWinner
	r.mu.RUnlock()
	return
}

func (r *Room) LastGameScores() (result []FinalScore) {
	r.mu.RLock()
	result = append([]FinalScore(nil), r.lastGameScores...)
	r.mu.RUnlock()
	return
}

func (r *Room) ReviewTitle() (result string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.state == StateReview && r.reviewWinner != nil {
		if r.reviewGameWinner {
			result = fmt.Sprintf("%s won the game!", r.reviewWinner.Nickname)
		} else {
			result = fmt.Sprintf("%s won the round!", r.reviewWinner.Nickname)
		}
	}
	return
}

func (r *Room) ReviewCountdown() (result int) {
	r.mu.RLock()
	deadline := r.reviewDeadline
	state := r.state
	r.mu.RUnlock()
	if state == StateReview && !deadline.IsZero() {
		remaining := time.Until(deadline)
		if remaining > 0 {
			result = int((remaining + time.Second - time.Nanosecond) / time.Second)
		}
	}
	return
}

func (r *Room) ReviewDeadlineUnixMilli() (result int64) {
	r.mu.RLock()
	deadline := r.reviewDeadline
	r.mu.RUnlock()
	if !deadline.IsZero() {
		result = deadline.UnixMilli()
	}
	return
}

func (r *Room) ReviewButtonBase() (result string) {
	r.mu.RLock()
	result = r.reviewButtonBaseLocked()
	r.mu.RUnlock()
	return
}

func (r *Room) ReviewProceedLabel() (result string) {
	result = r.ReviewButtonBase()
	countdown := r.ReviewCountdown()
	if countdown > 0 {
		result = fmt.Sprintf("%s (%d)", result, countdown)
	}
	return
}

func (r *Room) ReviewWaitingText() (result string) {
	base := r.ReviewButtonBase()
	countdown := r.ReviewCountdown()
	switch {
	case base == "":
		return
	case countdown <= 0 && base == "Back to Lobby":
		result = "Returning to the lobby."
		return
	case countdown <= 0:
		result = "Advancing to the next round."
		return
	case base == "Back to Lobby":
		result = fmt.Sprintf("Returning to the lobby in %d seconds.", countdown)
		return
	default:
		result = fmt.Sprintf("Next round in %d seconds.", countdown)
		return
	}
}

func (r *Room) IsRoundWinner(player *Player) (result bool) {
	r.mu.RLock()
	result = r.state == StateReview && player != nil && r.reviewWinner == player
	r.mu.RUnlock()
	return
}

func (r *Room) IsWinningSubmission(submission *Submission) (result bool) {
	r.mu.RLock()
	result = r.state == StateReview && submission != nil && r.reviewSubmission == submission
	r.mu.RUnlock()
	return
}

func (r *Room) SetPrivate(player *Player, private bool) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	err = r.setPrivateLocked(player, private)
	return
}

func (r *Room) SetNickname(player *Player, nickname string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.playerLocked(player)
	if current != nil {
		current.Nickname = NormalizeNickname(nickname)
		current.NicknameInput = current.Nickname
		current.Nickname = r.uniqueNicknameLocked(current)
		current.NicknameInput = current.Nickname
	}
}

func (r *Room) SetTargetScore(player *Player, score int) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	err = r.setTargetScoreLocked(player, score)
	return
}

func (r *Room) SetDeckEnabled(player *Player, selectedDeck *deck.Deck, enabled bool) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.host != player {
		err = ErrOnlyHostCanEdit
		return
	}
	if r.state != StateLobby {
		err = ErrDecksLocked
		return
	}
	if selectedDeck == nil || r.catalog.DeckByID(selectedDeck.ID) != selectedDeck {
		err = ErrUnknownDeck
		return
	}
	selected := make(map[*deck.Deck]bool, len(r.selectedDecks))
	for _, chosen := range r.selectedDecks {
		selected[chosen] = true
	}
	if enabled {
		selected[selectedDeck] = true
	} else {
		delete(selected, selectedDeck)
	}
	r.selectedDecks = sortedSelectedDecks(selected)
	return
}

func (r *Room) Start(player *Player) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.host != player {
		err = ErrOnlyHostCanStart
		return
	}
	if len(r.players) < r.minPlayers {
		err = fmt.Errorf("need at least %d players to start", r.minPlayers)
		return
	}
	blackCount, whiteCount := r.catalog.UnionCounts(r.selectedDecks)
	if blackCount < MinBlackCards {
		err = ErrNotEnoughBlackCards
		return
	}
	if whiteCount < MinWhiteCardsPerPlayer*len(r.players) {
		err = ErrNotEnoughWhiteCards
		return
	}
	blackCards, whiteCards := r.catalog.UnionCards(r.selectedDecks)
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
	r.round = 0
	r.czarIndex = -1
	for _, current := range r.players {
		current.Score = 0
		current.Hand = nil
		current.Submitted = nil
	}
	r.advanceRoundLocked()
	return
}

func (r *Room) PlayCards(player *Player, cards []*deck.WhiteCard) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.playerLocked(player)
	if current == nil {
		err = ErrOnlyPlayersCanPlay
		return
	}
	if r.state != StatePlaying {
		err = ErrNotYourTurn
		return
	}
	if r.judgeLocked() == current {
		err = ErrJudgeCannotPlay
		return
	}
	if len(current.Submitted) > 0 {
		err = ErrAlreadySubmitted
		return
	}
	cards = normalizeWhiteCards(cards)
	if len(cards) != r.currentBlackLocked().Pick {
		err = ErrNeedExactCards
		return
	}
	handSet := make(map[*deck.WhiteCard]struct{}, len(current.Hand))
	for _, card := range current.Hand {
		handSet[card] = struct{}{}
	}
	for _, card := range cards {
		if _, ok := handSet[card]; !ok {
			err = ErrCardNotInHand
			return
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
	}
	return
}

func (r *Room) Judge(player *Player, submission *Submission) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != StateJudging {
		err = ErrNotYourTurn
		return
	}
	if r.judgeLocked() != player {
		err = ErrNotJudge
		return
	}
	var winner *Player
	for _, candidate := range r.submissions {
		if candidate == submission {
			winner = candidate.Player
			break
		}
	}
	if winner == nil {
		err = ErrSubmissionNotFound
		return
	}
	winner.Score++
	r.lastWinnerName = winner.Nickname
	gameWinner := winner.Score >= r.targetScore
	if gameWinner {
		r.captureLastGameLocked(winner)
	}
	r.beginReviewLocked(winner, submission, gameWinner)
	return
}

func (r *Room) ProceedReview(player *Player) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != StateReview {
		err = ErrReviewNotReady
		return
	}
	if r.judgeLocked() != player {
		err = ErrNotJudge
		return
	}
	r.finishReviewLocked()
	return
}

func (r *Room) join(player *Player) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.playerLocked(player) == nil {
		if err = r.canJoinLocked(player); err == nil {
			r.seatLocked(player)
			r.players = append(r.players, player)
			if r.host == nil {
				r.host = player
			}
			r.dealJoinedPlayerLocked(player)
		}
	}
	return
}

func (r *Room) leave(player *Player) (empty bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.playerLocked(player)
	if current != nil {
		wasJudge := r.judgeLocked() == current
		idx := slices.Index(r.players, current)
		if idx >= 0 {
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
			r.submissions = slices.DeleteFunc(r.submissions, func(sub *Submission) (result bool) { result = sub.Player == current; return })
			if r.host == current {
				if len(r.players) > 0 {
					r.host = r.players[0]
				} else {
					r.host = nil
				}
			}
			if len(r.players) > 0 {
				if r.state != StateLobby {
					switch {
					case len(r.players) < r.minPlayers:
						r.resetToLobbyLocked()
					case wasJudge:
						r.resetToLobbyLocked()
					case len(r.submissions) == len(r.players)-1 && r.state == StatePlaying:
						r.rand.Shuffle(len(r.submissions), func(i, j int) { r.submissions[i], r.submissions[j] = r.submissions[j], r.submissions[i] })
						r.state = StateJudging
					}
				}
			}
		}
	}
	return len(r.players) == 0
}

func (r *Room) expiredPlayers() (result []*Player) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result = make([]*Player, 0)
	for _, player := range r.players {
		if player == nil || player.Session == nil || player.Session.Cookie().MaxAge < 0 {
			result = append(result, player)
		}
	}
	return
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

func (r *Room) canJoinLocked(player *Player) (err error) {
	if player == nil {
		err = ErrRoomNotFound
		return
	}
	if player.Room != nil {
		err = ErrAlreadyInRoom
		return
	}
	if len(r.players) >= MaxPlayers {
		err = ErrRoomFull
		return
	}
	if r.state == StateLobby {
		return
	}
	_, whiteCount := r.catalog.UnionCounts(r.selectedDecks)
	if whiteCount < MinWhiteCardsPerPlayer*(len(r.players)+1) {
		err = ErrNotEnoughWhiteCards
	}
	return
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

func (r *Room) uniqueNicknameLocked(player *Player) (result string) {
	result = NormalizeNickname(player.NicknameInput)
	if result == "Player" && strings.TrimSpace(player.Nickname) != "" {
		result = NormalizeNickname(player.Nickname)
	}
	base := result
	for suffix := 2; ; suffix++ {
		if !r.nicknameTakenLocked(result, player) {
			return
		}
		result = fmt.Sprintf("%s-%d", base, suffix)
	}
}

func (r *Room) nicknameTakenLocked(candidate string, exclude *Player) (result bool) {
	for _, player := range r.players {
		if player == exclude {
			continue
		}
		if strings.EqualFold(player.Nickname, candidate) {
			result = true
			return
		}
	}
	return
}

func (r *Room) resetToLobbyLocked() {
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
	slices.SortStableFunc(r.lastGameScores, func(a, b FinalScore) (result int) {
		if a.Score != b.Score {
			result = b.Score - a.Score
			return
		}
		result = strings.Compare(a.Nickname, b.Nickname)
		return

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
		r.resetToLobbyLocked()
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
	if r.reviewGameWinner {
		r.resetToLobbyLocked()
		return
	}
	r.advanceRoundLocked()
}

func (r *Room) autoProceedReview(token uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state == StateReview && r.reviewToken == token {
		r.finishReviewLocked()
		manager := r.manager
		if manager != nil {
			manager.notify(r)
		}
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

func (r *Room) reviewButtonBaseLocked() (result string) {
	if r.state == StateReview {
		if r.reviewGameWinner {
			result = "Back to Lobby"
		} else {
			result = "Next Round"
		}
	}
	return
}

func (r *Room) drawWhiteLocked() (result *deck.WhiteCard) {
	if len(r.whiteDraw) == 0 {
		r.whiteDraw = append(r.whiteDraw, r.whiteDiscard...)
		r.whiteDiscard = nil
		r.rand.Shuffle(len(r.whiteDraw), func(i, j int) { r.whiteDraw[i], r.whiteDraw[j] = r.whiteDraw[j], r.whiteDraw[i] })
	}
	result = r.whiteDraw[len(r.whiteDraw)-1]
	r.whiteDraw = r.whiteDraw[:len(r.whiteDraw)-1]
	return
}

func (r *Room) drawBlackLocked() (result *deck.BlackCard) {
	if len(r.blackDraw) == 0 {
		r.blackDraw = append(r.blackDraw, r.blackDiscard...)
		r.blackDiscard = nil
		r.rand.Shuffle(len(r.blackDraw), func(i, j int) { r.blackDraw[i], r.blackDraw[j] = r.blackDraw[j], r.blackDraw[i] })
	}
	result = r.blackDraw[len(r.blackDraw)-1]
	r.blackDraw = r.blackDraw[:len(r.blackDraw)-1]
	return
}

func (r *Room) judgeLocked() (result *Player) {
	if len(r.players) == 0 || r.czarIndex < 0 || r.czarIndex >= len(r.players) {
		return
	}
	result = r.players[r.czarIndex]
	return
}

func (r *Room) currentBlackLocked() (result *deck.BlackCard) {
	result = r.currentBlack
	return
}

func (r *Room) playerLocked(player *Player) (result *Player) {
	for _, current := range r.players {
		if current == player {
			result = current
			return
		}
	}
	return
}

func (r *Room) SelectedDecksForWhiteCard(card *deck.WhiteCard) (result []*deck.Deck) {
	if r != nil && card != nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		result = r.selectedDecksForWhiteCardLocked(card)
	}
	return
}

func (r *Room) SelectedDecksForBlackCard(card *deck.BlackCard) (result []*deck.Deck) {
	if r != nil && card != nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		result = r.selectedDecksForBlackCardLocked(card)
	}
	return
}

func (r *Room) FirstSelectedDeckNameForWhiteCard(card *deck.WhiteCard) (result string) {
	if r != nil && card != nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		result = r.firstSelectedDeckNameForWhiteCardLocked(card)
	}
	return
}

func (r *Room) FirstSelectedDeckNameForBlackCard(card *deck.BlackCard) (result string) {
	if r != nil && card != nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		result = r.firstSelectedDeckNameForBlackCardLocked(card)
	}
	return
}

func (r *Room) selectedDecksForWhiteCardLocked(card *deck.WhiteCard) (result []*deck.Deck) {
	result = make([]*deck.Deck, 0, len(r.selectedDecks))
	for _, d := range r.selectedDecks {
		if slices.Contains(d.WhiteCards, card) {
			result = append(result, d)
		}
	}
	return
}

func (r *Room) selectedDecksForBlackCardLocked(card *deck.BlackCard) (result []*deck.Deck) {
	result = make([]*deck.Deck, 0, len(r.selectedDecks))
	for _, d := range r.selectedDecks {
		if slices.Contains(d.BlackCards, card) {
			result = append(result, d)
		}
	}
	return
}

func (r *Room) firstSelectedDeckNameForWhiteCardLocked(card *deck.WhiteCard) (result string) {
	for _, d := range r.selectedDecks {
		if slices.Contains(d.WhiteCards, card) {
			result = d.Name
			return
		}
	}
	return
}

func (r *Room) firstSelectedDeckNameForBlackCardLocked(card *deck.BlackCard) (result string) {
	for _, d := range r.selectedDecks {
		if slices.Contains(d.BlackCards, card) {
			result = d.Name
			return
		}
	}
	return
}

func (r *Room) setTargetScoreLocked(player *Player, score int) (err error) {
	if score < r.minTargetScoreLocked() {
		score = r.minTargetScoreLocked()
	} else if score > 10 {
		score = 10
	}
	if r.host != player {
		err = ErrOnlyHostCanEdit
		return
	}
	if r.state != StateLobby {
		err = ErrGameInProgress
		return
	}
	r.targetScore = score
	return
}

func (r *Room) setPrivateLocked(player *Player, private bool) (err error) {
	if r.host != player {
		err = ErrOnlyHostCanEdit
		return
	}
	if r.state != StateLobby {
		err = ErrGameInProgress
		return
	}
	r.private = private
	return
}

func (r *Room) minTargetScoreLocked() (result int) {
	if r.debug {
		result = 1
		return
	}
	result = 2
	return
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

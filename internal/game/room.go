package game

import (
	"fmt"
	mathrand "math/rand"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/linkdata/xyzzy/internal/deck"
)

// RoomState identifies a room's current phase.
type RoomState string

const (
	// StateLobby accepts players and lobby-setting changes.
	StateLobby RoomState = "lobby"
	// StatePlaying accepts card submissions from players other than the judge.
	StatePlaying RoomState = "playing"
	// StateJudging accepts the judge's winning submission.
	StateJudging RoomState = "judging"
	// StateReview displays the round result before the next state begins.
	StateReview RoomState = "results"
)

// Room coordinates one synchronized game and its seated players.
//
// Its methods may be called concurrently. Rooms are created by [Manager]; the
// zero value is not ready for use.
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

// Code returns the room's immutable join code.
func (r *Room) Code() string { return r.code }

// State returns the room's current phase.
func (r *Room) State() (result RoomState) {
	r.mu.RLock()
	result = r.state
	r.mu.RUnlock()
	return
}

// Host returns the current host, or nil when the room is empty.
func (r *Room) Host() (result *Player) {
	r.mu.RLock()
	result = r.host
	r.mu.RUnlock()
	return
}

// HostName returns the current host's nickname.
//
// It returns an empty string when the room has no host.
func (r *Room) HostName() (result string) {
	r.mu.RLock()
	if r.host != nil {
		result = r.host.Nickname
	}
	r.mu.RUnlock()
	return
}

// Players returns a shallow copy of the seated-player list.
func (r *Room) Players() (result []*Player) {
	r.mu.RLock()
	result = append([]*Player(nil), r.players...)
	r.mu.RUnlock()
	return
}

// PlayerCount returns the number of seated players.
func (r *Room) PlayerCount() (result int) {
	r.mu.RLock()
	result = len(r.players)
	r.mu.RUnlock()
	return
}

// ScoreFor returns a seated player's current score.
//
// It returns zero when player is nil or not seated in the room.
func (r *Room) ScoreFor(player *Player) (result int) {
	r.mu.RLock()
	if current := r.playerLocked(player); current != nil {
		result = current.Score
	}
	r.mu.RUnlock()
	return
}

// SubmittedBy reports whether a seated player has submitted this round.
func (r *Room) SubmittedBy(player *Player) (result bool) {
	r.mu.RLock()
	if current := r.playerLocked(player); current != nil {
		result = len(current.Submitted) > 0
	}
	r.mu.RUnlock()
	return
}

// HasPlayer reports whether player is seated in the room.
func (r *Room) HasPlayer(player *Player) (result bool) {
	r.mu.RLock()
	result = r.playerLocked(player) != nil
	r.mu.RUnlock()
	return
}

// IsHost reports whether player is the current host.
func (r *Room) IsHost(player *Player) (result bool) {
	r.mu.RLock()
	result = player != nil && r.host == player
	r.mu.RUnlock()
	return
}

// IsJudge reports whether player is the current judge in an active game.
func (r *Room) IsJudge(player *Player) (result bool) {
	r.mu.RLock()
	result = player != nil && r.state != StateLobby && r.judgeLocked() == player
	r.mu.RUnlock()
	return
}

// CanJoin reports whether player can currently join the room.
func (r *Room) CanJoin(player *Player) (result bool) {
	r.mu.RLock()
	result = r.canJoinLocked(player) == nil
	r.mu.RUnlock()
	return
}

// CanSubmit reports whether player can currently submit cards.
func (r *Room) CanSubmit(player *Player) (result bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	current := r.playerLocked(player)
	result = current != nil && r.canSubmitLocked(current)
	return
}

// CanJudge reports whether player can currently choose the winning submission.
func (r *Room) CanJudge(player *Player) (result bool) {
	r.mu.RLock()
	result = player != nil && r.state == StateJudging && r.judgeLocked() == player
	r.mu.RUnlock()
	return
}

// DeckEnabled reports whether d is selected for the room.
func (r *Room) DeckEnabled(d *deck.Deck) (result bool) {
	if d != nil {
		r.mu.RLock()
		result = slices.Contains(r.selectedDecks, d)
		r.mu.RUnlock()
	}
	return
}

// BlackCount returns the number of black cards in the selected decks.
func (r *Room) BlackCount() (result int) {
	r.mu.RLock()
	result, _ = r.catalog.UnionCounts(r.selectedDecks)
	r.mu.RUnlock()
	return
}

// WhiteCount returns the number of white cards in the selected decks.
func (r *Room) WhiteCount() (result int) {
	r.mu.RLock()
	_, result = r.catalog.UnionCounts(r.selectedDecks)
	r.mu.RUnlock()
	return
}

// RequiredWhite returns the minimum white-card count for the current players.
func (r *Room) RequiredWhite() (result int) {
	r.mu.RLock()
	result = MinWhiteCardsPerPlayer * max(len(r.players), 1)
	r.mu.RUnlock()
	return
}

// TargetScore returns the number of points required to win the game.
func (r *Room) TargetScore() (result int) {
	r.mu.RLock()
	result = r.targetScore
	r.mu.RUnlock()
	return
}

// IsPrivate reports whether the room is omitted from the public room list.
func (r *Room) IsPrivate() (result bool) {
	r.mu.RLock()
	result = r.private
	r.mu.RUnlock()
	return
}

// MinTargetScore returns the smallest accepted target score.
func (r *Room) MinTargetScore() (result int) {
	r.mu.RLock()
	result = r.minTargetScoreLocked()
	r.mu.RUnlock()
	return
}

// CurrentBlack returns the current black card, or nil outside an active round.
func (r *Room) CurrentBlack() (result *deck.BlackCard) {
	r.mu.RLock()
	result = r.currentBlackLocked()
	r.mu.RUnlock()
	return
}

// NeedPick returns the number of white cards required by the current prompt.
//
// It returns zero outside an active round.
func (r *Room) NeedPick() (result int) {
	r.mu.RLock()
	if black := r.currentBlackLocked(); black != nil {
		result = black.Pick
	}
	r.mu.RUnlock()
	return
}

// HandFor returns a shallow copy of a seated player's hand.
//
// It returns nil when player is nil or not seated in the room.
func (r *Room) HandFor(player *Player) (cards []*deck.WhiteCard) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if current := r.playerLocked(player); current != nil {
		cards = append(cards, current.Hand...)
	}
	return
}

// NicknameFor returns a seated player's room-visible nickname.
//
// It returns an empty string when player is nil or not seated in the room.
func (r *Room) NicknameFor(player *Player) (result string) {
	r.mu.RLock()
	if current := r.playerLocked(player); current != nil {
		result = current.Nickname
	}
	r.mu.RUnlock()
	return
}

// SelectionOrderFor returns card's one-based selection order for player.
//
// It returns zero when the player, card, or selection is absent.
func (r *Room) SelectionOrderFor(player *Player, card *deck.WhiteCard) (result int) {
	r.mu.RLock()
	if current := r.playerLocked(player); current != nil {
		result = selectionOrderLocked(current, card)
	}
	r.mu.RUnlock()
	return
}

// ToggleCardSelection toggles card in player's current submission selection.
//
// It reports whether the selection changed. Invalid cards and players that
// cannot currently submit leave the selection unchanged.
func (r *Room) ToggleCardSelection(player *Player, card *deck.WhiteCard) (changed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.playerLocked(player)
	if current == nil || !r.canSubmitLocked(current) || card == nil || !slices.Contains(current.Hand, card) {
		return
	}
	needPick := r.needPickLocked()
	if idx := slices.Index(current.SelectedCards, card); idx >= 0 {
		current.SelectedCards = slices.Delete(current.SelectedCards, idx, idx+1)
		changed = true
		return
	}
	if needPick == 1 {
		current.SelectedCards = []*deck.WhiteCard{card}
		changed = true
		return
	}
	if len(current.SelectedCards) >= needPick {
		return
	}
	current.SelectedCards = append(current.SelectedCards, card)
	changed = true
	return
}

// SubmissionSelected reports whether submission is selected by player.
func (r *Room) SubmissionSelected(player *Player, submission *Submission) (result bool) {
	r.mu.RLock()
	if current := r.playerLocked(player); current != nil {
		result = submission != nil && current.SelectedSubmission == submission
	}
	r.mu.RUnlock()
	return
}

// ToggleSubmissionSelection toggles submission in the judge's current selection.
//
// It reports whether the selection changed. Unknown submissions and players
// that cannot currently judge leave the selection unchanged.
func (r *Room) ToggleSubmissionSelection(player *Player, submission *Submission) (changed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.playerLocked(player)
	if current == nil || r.state != StateJudging || r.judgeLocked() != current || submission == nil || !slices.Contains(r.submissions, submission) {
		return
	}
	if current.SelectedSubmission == submission {
		current.SelectedSubmission = nil
	} else {
		current.SelectedSubmission = submission
	}
	changed = true
	return
}

// Submissions returns a shallow copy of the current round's submissions.
func (r *Room) Submissions() (submissions []*Submission) {
	r.mu.RLock()
	submissions = append(submissions, r.submissions...)
	r.mu.RUnlock()
	return
}

// SubmissionCards returns a copy of submission's card slice.
//
// It returns nil for a nil submission.
func (r *Room) SubmissionCards(submission *Submission) (cards []*deck.WhiteCard) {
	if submission != nil {
		r.mu.RLock()
		cards = append(cards, submission.Cards...)
		r.mu.RUnlock()
	}
	return
}

// JudgePlayer returns the current judge, or nil when no judge is assigned.
func (r *Room) JudgePlayer() (result *Player) {
	r.mu.RLock()
	result = r.judgeLocked()
	r.mu.RUnlock()
	return
}

// JudgeName returns the current judge's nickname.
//
// It returns an empty string when there is no judge.
func (r *Room) JudgeName() (result string) {
	r.mu.RLock()
	if judge := r.judgeLocked(); judge != nil {
		result = judge.Nickname
	}
	r.mu.RUnlock()
	return
}

// LastGameWinner returns the captured nickname of the most recent game winner.
//
// It returns an empty string until a game has completed or after a new game starts.
func (r *Room) LastGameWinner() (result string) {
	r.mu.RLock()
	result = r.lastGameWinner
	r.mu.RUnlock()
	return
}

// LastGameScores returns a copy of the most recently completed game's scores.
func (r *Room) LastGameScores() (result []FinalScore) {
	r.mu.RLock()
	result = append([]FinalScore(nil), r.lastGameScores...)
	r.mu.RUnlock()
	return
}

// IsRoundWinner reports whether player won the round currently under review.
func (r *Room) IsRoundWinner(player *Player) (result bool) {
	r.mu.RLock()
	result = r.state == StateReview && player != nil && r.reviewWinner == player
	r.mu.RUnlock()
	return
}

// IsWinningSubmission reports whether submission won the round under review.
func (r *Room) IsWinningSubmission(submission *Submission) (result bool) {
	r.mu.RLock()
	result = r.state == StateReview && submission != nil && r.reviewSubmission == submission
	r.mu.RUnlock()
	return
}

// SetPrivate changes whether the room appears in the public room list.
//
// Only the host may change this setting while the room is in [StateLobby].
func (r *Room) SetPrivate(player *Player, private bool) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	err = r.setPrivateLocked(player, private)
	return
}

func (r *Room) setNickname(player *Player, nickname string) (changed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.playerLocked(player)
	if current != nil {
		nickname = r.uniqueNicknameForLocked(current, NormalizeNickname(nickname))
		changed = current.setNickname(nickname)
	}
	return
}

// SetTargetScore changes the number of points required to win.
//
// Only the host may change this setting while the room is in [StateLobby]. The
// value is clamped to [Room.MinTargetScore] through 10.
func (r *Room) SetTargetScore(player *Player, score int) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	err = r.setTargetScoreLocked(player, score)
	return
}

// SetDeckEnabled updates the room's deck selection.
//
// It reports whether the selection changed. Only the host can change a deck
// from the room's catalog while the room is in [StateLobby].
func (r *Room) SetDeckEnabled(player *Player, selectedDeck *deck.Deck, enabled bool) (changed bool, err error) {
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
	if slices.Contains(r.selectedDecks, selectedDeck) == enabled {
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
	changed = true
	return
}

// Start begins a game and deals its opening round.
//
// The caller must be the host of a lobby with enough players and enough cards
// in its selected decks.
func (r *Room) Start(player *Player) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.host != player {
		err = ErrOnlyHostCanStart
		return
	}
	if r.state != StateLobby {
		err = ErrGameInProgress
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
	r.lastGameWinner = ""
	r.lastGameScores = nil
	r.round = 0
	r.czarIndex = -1
	for _, current := range r.players {
		current.Score = 0
		current.Hand = nil
		current.Submitted = nil
		current.SelectedCards = nil
		current.SelectedSubmission = nil
	}
	r.advanceRoundLocked()
	return
}

// PlayCards submits cards from player's hand for the current prompt.
//
// The card slice is copied on success. The submission must contain exactly the
// required number of unique cards, and the judge cannot submit.
func (r *Room) PlayCards(player *Player, cards []*deck.WhiteCard) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.playerLocked(player)
	err = r.playCardsLocked(current, cards)
	return
}

// PlaySelectedCards submits and clears the player's current card selection.
//
// The selection is cleared only after a successful submission.
func (r *Room) PlaySelectedCards(player *Player) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.playerLocked(player)
	if current != nil {
		err = r.playCardsLocked(current, current.SelectedCards)
		if err == nil {
			current.SelectedCards = nil
		}
	} else {
		err = ErrOnlyPlayersCanPlay
	}
	return
}

func (r *Room) playCardsLocked(current *Player, cards []*deck.WhiteCard) (err error) {
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

// Judge records submission as the winner of the current round.
//
// The caller must be the current judge and submission must belong to the round.
func (r *Room) Judge(player *Player, submission *Submission) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	err = r.judgeSubmissionLocked(player, submission)
	return
}

// JudgeSelectedSubmission records and clears the judge's selected submission.
//
// The selection is cleared only after a successful result.
func (r *Room) JudgeSelectedSubmission(player *Player) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.playerLocked(player)
	if current == nil {
		err = ErrNotJudge
		return
	}
	submission := current.SelectedSubmission
	if err = r.judgeSubmissionLocked(player, submission); err == nil {
		current.SelectedSubmission = nil
	}
	return
}

func (r *Room) judgeSubmissionLocked(player *Player, submission *Submission) (err error) {
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
	gameWinner := winner.Score >= r.targetScore
	if gameWinner {
		r.captureLastGameLocked(winner)
	}
	r.beginReviewLocked(winner, submission, gameWinner)
	return
}

// ProceedReview advances a completed round immediately.
//
// The caller must be the current judge while the room is in [StateReview]. It
// starts the next round or returns a completed game to the lobby.
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
		wasReviewWinner := r.state == StateReview && r.reviewWinner == current
		idx := slices.Index(r.players, current)
		if idx >= 0 {
			if idx < r.czarIndex {
				r.czarIndex--
			}
			r.whiteDiscard = append(r.whiteDiscard, current.Hand...)
			r.whiteDiscard = append(r.whiteDiscard, current.Submitted...)
			current.setRoom(nil)
			current.Score = 0
			current.Hand = nil
			current.Submitted = nil
			current.SelectedCards = nil
			current.SelectedSubmission = nil
			r.players = append(r.players[:idx], r.players[idx+1:]...)
			r.submissions = slices.DeleteFunc(r.submissions, func(sub *Submission) (result bool) { result = sub.Player == current; return })
			for _, other := range r.players {
				if other.SelectedSubmission != nil && other.SelectedSubmission.Player == current {
					other.SelectedSubmission = nil
				}
			}
			if r.host == current {
				if len(r.players) > 0 {
					r.host = r.players[0]
				} else {
					r.host = nil
				}
			}
			if len(r.players) == 0 {
				r.clearReviewLocked()
			} else if r.state != StateLobby {
				switch {
				case len(r.players) < r.minPlayers:
					r.resetToLobbyLocked()
				case wasJudge:
					r.resetToLobbyLocked()
				case wasReviewWinner:
					r.finishReviewLocked()
				case len(r.submissions) == len(r.players)-1 && r.state == StatePlaying:
					r.rand.Shuffle(len(r.submissions), func(i, j int) { r.submissions[i], r.submissions[j] = r.submissions[j], r.submissions[i] })
					r.state = StateJudging
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
	player.setNickname(r.uniqueNicknameLocked(player))
	player.setRoom(r)
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
	if player.Room() != nil {
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
		card := r.drawWhiteLocked()
		if card == nil {
			break
		}
		player.Hand = append(player.Hand, card)
	}
	if r.state != StatePlaying {
		return
	}
	black := r.currentBlackLocked()
	if black == nil {
		return
	}
	for i := 0; i < black.Draw; i++ {
		card := r.drawWhiteLocked()
		if card == nil {
			break
		}
		player.Hand = append(player.Hand, card)
	}
}

func (r *Room) uniqueNicknameLocked(player *Player) (result string) {
	result = NormalizeNickname(player.NicknameInputValue())
	nickname := player.NicknameValue()
	if result == "Player" && strings.TrimSpace(nickname) != "" {
		result = NormalizeNickname(nickname)
	}
	result = r.uniqueNicknameForLocked(player, result)
	return
}

func (r *Room) uniqueNicknameForLocked(player *Player, nickname string) (result string) {
	result = nickname
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
			card := r.drawWhiteLocked()
			if card == nil {
				break
			}
			player.Hand = append(player.Hand, card)
		}
	}
	r.currentBlack = r.drawBlackLocked()
	if r.currentBlack == nil {
		r.resetToLobbyLocked()
		return
	}
	black := r.currentBlackLocked()
	judge := r.judgeLocked()
	for _, player := range r.players {
		if player == judge {
			continue
		}
		for i := 0; black != nil && i < black.Draw; i++ {
			card := r.drawWhiteLocked()
			if card == nil {
				break
			}
			player.Hand = append(player.Hand, card)
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
	r.scheduleReviewTimerLocked()
}

func (r *Room) finishReviewLocked() {
	if r.reviewGameWinner {
		r.resetToLobbyLocked()
		return
	}
	r.advanceRoundLocked()
}

func (r *Room) scheduleReviewTimerLocked() {
	delay := reviewTimerDelay(time.Now(), r.reviewDeadline)
	token := r.reviewToken
	r.reviewTimer = time.AfterFunc(delay, func() {
		r.reviewTimerElapsed(token)
	})
}

func reviewTimerDelay(now, deadline time.Time) (result time.Duration) {
	remaining := deadline.Sub(now)
	switch {
	case remaining <= 0:
	case remaining <= time.Second:
		result = remaining
	default:
		result = remaining % time.Second
		if result == 0 {
			result = time.Second
		}
	}
	return
}

func (r *Room) reviewTimerElapsed(token uint64) {
	var manager *Manager
	var dirty any
	r.mu.Lock()
	if r.state == StateReview && r.reviewToken == token {
		manager = r.manager
		if time.Now().Before(r.reviewDeadline) {
			r.scheduleReviewTimerLocked()
			dirty = &r.reviewDeadline
		} else {
			r.finishReviewLocked()
			dirty = r
		}
	}
	r.mu.Unlock()
	if manager != nil && dirty != nil {
		manager.notify(dirty)
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

func (r *Room) drawWhiteLocked() (result *deck.WhiteCard) {
	if len(r.whiteDraw) == 0 {
		r.whiteDraw = append(r.whiteDraw, r.whiteDiscard...)
		r.whiteDiscard = nil
		r.rand.Shuffle(len(r.whiteDraw), func(i, j int) { r.whiteDraw[i], r.whiteDraw[j] = r.whiteDraw[j], r.whiteDraw[i] })
	}
	if len(r.whiteDraw) == 0 {
		return
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
	if len(r.blackDraw) == 0 {
		return
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

func (r *Room) canSubmitLocked(player *Player) (result bool) {
	result = player != nil && r.state == StatePlaying && r.judgeLocked() != player && len(player.Submitted) == 0
	return
}

func (r *Room) needPickLocked() (result int) {
	if black := r.currentBlackLocked(); black != nil {
		result = black.Pick
	}
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

func selectionOrderLocked(player *Player, card *deck.WhiteCard) (result int) {
	for i, selected := range player.SelectedCards {
		if selected == card {
			result = i + 1
			return
		}
	}
	return
}

// FirstSelectedDeckNameForWhiteCard returns the first selected deck containing card.
//
// It returns an empty string for a nil room, a nil card, or a card absent from
// the selected decks.
func (r *Room) FirstSelectedDeckNameForWhiteCard(card *deck.WhiteCard) (result string) {
	if r != nil && card != nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		result = r.firstSelectedDeckNameForWhiteCardLocked(card)
	}
	return
}

// FirstSelectedDeckNameForBlackCard returns the first selected deck containing card.
//
// It returns an empty string for a nil room, a nil card, or a card absent from
// the selected decks.
func (r *Room) FirstSelectedDeckNameForBlackCard(card *deck.BlackCard) (result string) {
	if r != nil && card != nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		result = r.firstSelectedDeckNameForBlackCardLocked(card)
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
	score = r.normalizeTargetScoreLocked(score)
	if err = r.lobbyControlErrorLocked(player); err == nil {
		r.targetScore = score
	}
	return
}

func (r *Room) setPrivateLocked(player *Player, private bool) (err error) {
	if err = r.lobbyControlErrorLocked(player); err == nil {
		r.private = private
	}
	return
}

func (r *Room) lobbyControlErrorLocked(player *Player) (err error) {
	if player == nil || r.host != player {
		err = ErrOnlyHostCanEdit
		return
	}
	if r.state != StateLobby {
		err = ErrGameInProgress
		return
	}
	return
}

func (r *Room) normalizeTargetScoreLocked(score int) (result int) {
	result = score
	if result < r.minTargetScoreLocked() {
		result = r.minTargetScoreLocked()
	} else if result > 10 {
		result = 10
	}
	return
}

func (r *Room) canStartLocked(player *Player) (result bool) {
	if player == nil || r.host != player || r.state != StateLobby || len(r.players) < r.minPlayers {
		return
	}
	blackCount, whiteCount := r.catalog.UnionCounts(r.selectedDecks)
	result = blackCount >= MinBlackCards && whiteCount >= MinWhiteCardsPerPlayer*len(r.players)
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

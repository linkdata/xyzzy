package game

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	mathrand "math/rand"
	"slices"
	"strings"
	"sync"

	"github.com/linkdata/xyzzy/internal/deck"
)

const (
	MinPlayers             = 3
	MaxPlayers             = 10
	HandSize               = 10
	ScoreGoal              = 5
	MinBlackCards          = 50
	MinWhiteCardsPerPlayer = 20
	roomCodeLength         = 6
	roomCodeAlphabet       = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

var (
	ErrRoomNotFound        = errors.New("room not found")
	ErrRoomFull            = errors.New("room is full")
	ErrGameInProgress      = errors.New("game already in progress")
	ErrAlreadyInRoom       = errors.New("player is already in a room")
	ErrOnlyHostCanEdit     = errors.New("only the host can change deck selection")
	ErrDecksLocked         = errors.New("deck selection is locked after the game starts")
	ErrUnknownDeck         = errors.New("unknown deck")
	ErrNotEnoughBlackCards = errors.New("selected decks need at least 50 unique black cards")
	ErrNotEnoughWhiteCards = errors.New("selected decks need at least 20 white cards per player")
	ErrOnlyHostCanStart    = errors.New("only the host can start the game")
	ErrOnlyPlayersCanPlay  = errors.New("only players in the room can play")
	ErrNotYourTurn         = errors.New("not your turn")
	ErrJudgeCannotPlay     = errors.New("judge cannot play cards")
	ErrNeedExactCards      = errors.New("must select the exact number of cards")
	ErrCardNotInHand       = errors.New("selected card is not in your hand")
	ErrAlreadySubmitted    = errors.New("cards already submitted")
	ErrSubmissionNotFound  = errors.New("submission not found")
	ErrNotJudge            = errors.New("only the judge can pick a winner")
)

func NewManager(catalog *deck.Catalog) *Manager {
	return NewManagerWithOptions(catalog, Options{})
}

func NewManagerWithOptions(catalog *deck.Catalog, opts Options) *Manager {
	if opts.MinPlayers < 2 {
		opts.MinPlayers = MinPlayers
	}
	return &Manager{
		rooms:   make(map[string]*Room),
		catalog: catalog,
		opts:    opts,
	}
}

func NormalizeNickname(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "Player"
	}
	return b.String()
}

func (m *Manager) CreateRoom(player *Player, defaultDeckIDs []string) (*Room, error) {
	if player == nil {
		return nil, ErrAlreadyInRoom
	}
	if player.Room != nil {
		return nil, ErrAlreadyInRoom
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	code, err := m.newRoomCodeLocked()
	if err != nil {
		return nil, err
	}
	roomRand, err := newCryptoRand()
	if err != nil {
		return nil, err
	}
	room := &Room{
		code:            code,
		catalog:         m.catalog,
		rand:            roomRand,
		minPlayers:      m.opts.MinPlayers,
		targetScore:     ScoreGoal,
		state:           StateLobby,
		czarIndex:       -1,
		selectedDeckIDs: normalizeDeckIDs(m.catalog, defaultDeckIDs),
	}
	room.seatLocked(player)
	room.host = player
	room.players = []*Player{player}
	m.rooms[code] = room
	return room, nil
}

func (m *Manager) Room(code string) *Room {
	m.mu.RLock()
	room := m.rooms[strings.ToUpper(strings.TrimSpace(code))]
	m.mu.RUnlock()
	return room
}

func (m *Manager) GetRoom(code string) *Room {
	return m.Room(code)
}

func (m *Manager) Rooms() []*Room {
	m.mu.RLock()
	rooms := make([]*Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		rooms = append(rooms, room)
	}
	m.mu.RUnlock()
	slices.SortFunc(rooms, func(a, b *Room) int { return strings.Compare(a.code, b.code) })
	return rooms
}

func (m *Manager) JoinRoom(code string, player *Player) (*Room, error) {
	if player == nil {
		return nil, ErrRoomNotFound
	}
	room := m.Room(code)
	if room == nil {
		return nil, ErrRoomNotFound
	}
	if player.Room == room {
		return room, nil
	}
	if player.Room != nil {
		return nil, ErrAlreadyInRoom
	}
	if err := room.join(player); err != nil {
		return nil, err
	}
	return room, nil
}

func (m *Manager) LeaveRoom(player *Player) (*Room, bool) {
	if player == nil || player.Room == nil {
		return nil, false
	}
	room := player.Room
	destroy := room.leave(player)
	if destroy {
		m.mu.Lock()
		if m.rooms[room.code] == room {
			delete(m.rooms, room.code)
		}
		m.mu.Unlock()
	}
	return room, destroy
}

func (m *Manager) CleanupExpiredSessions() []*Room {
	rooms := m.Rooms()
	affected := make([]*Room, 0)
	for _, room := range rooms {
		expired := room.expiredPlayers()
		if len(expired) == 0 {
			continue
		}
		affected = append(affected, room)
		for _, player := range expired {
			destroy := room.leave(player)
			if destroy {
				m.mu.Lock()
				if m.rooms[room.code] == room {
					delete(m.rooms, room.code)
				}
				m.mu.Unlock()
				break
			}
		}
	}
	return affected
}

func (m *Manager) newRoomCodeLocked() (string, error) {
	for i := 0; i < 1024; i++ {
		code, err := randomCode()
		if err != nil {
			return "", err
		}
		if _, exists := m.rooms[code]; !exists {
			return code, nil
		}
	}
	return "", errors.New("could not allocate room code")
}

func (r *Room) Code() string { return r.code }

func (r *Room) Locker() *sync.RWMutex { return &r.mu }

func (r *Room) TargetScorePtr() *int { return &r.targetScore }

func (p *Player) UILocker() *sync.Mutex { return &p.uiMu }

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
	canJoin := player != nil && player.Room == nil && r.state == StateLobby && len(r.players) < MaxPlayers
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
		cards = r.lookupWhiteCardsLocked(current.Hand)
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
	cards := r.lookupWhiteCardsLocked(submission.CardIDs)
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
	r.blackDraw = idsFromBlack(blackCards)
	r.whiteDraw = idsFromWhite(whiteCards)
	r.rand.Shuffle(len(r.blackDraw), func(i, j int) { r.blackDraw[i], r.blackDraw[j] = r.blackDraw[j], r.blackDraw[i] })
	r.rand.Shuffle(len(r.whiteDraw), func(i, j int) { r.whiteDraw[i], r.whiteDraw[j] = r.whiteDraw[j], r.whiteDraw[i] })
	r.blackDiscard = nil
	r.whiteDiscard = nil
	r.submissions = nil
	r.currentBlackID = ""
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

func (r *Room) PlayCards(player *Player, cardIDs []string) error {
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
	cardIDs = normalizeIDs(cardIDs)
	if len(cardIDs) != r.currentBlackLocked().Pick {
		return ErrNeedExactCards
	}
	handSet := make(map[string]int, len(current.Hand))
	for i, id := range current.Hand {
		handSet[id] = i
	}
	for _, cardID := range cardIDs {
		if _, ok := handSet[cardID]; !ok {
			return ErrCardNotInHand
		}
	}
	remaining := make([]string, 0, len(current.Hand)-len(cardIDs))
	selected := make(map[string]struct{}, len(cardIDs))
	for _, cardID := range cardIDs {
		selected[cardID] = struct{}{}
	}
	for _, cardID := range current.Hand {
		if _, ok := selected[cardID]; ok {
			continue
		}
		remaining = append(remaining, cardID)
	}
	current.Hand = remaining
	current.Submitted = append([]string(nil), cardIDs...)
	r.submissions = append(r.submissions, &Submission{
		ID:      submissionID(cardIDs),
		Player:  current,
		CardIDs: append([]string(nil), cardIDs...),
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
	if winner.Score >= r.targetScore {
		r.captureLastGameLocked(winner)
		r.resetToLobbyLocked(fmt.Sprintf("%s won the game. Room reset to the lobby.", winner.Nickname))
		return nil
	}
	r.statusMessage = fmt.Sprintf("%s won the round.", winner.Nickname)
	r.advanceRoundLocked()
	return nil
}

func (r *Room) join(player *Player) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.playerLocked(player) != nil {
		return nil
	}
	if len(r.players) >= MaxPlayers {
		return ErrRoomFull
	}
	if r.state != StateLobby {
		return ErrGameInProgress
	}
	r.seatLocked(player)
	r.players = append(r.players, player)
	if r.host == nil {
		r.host = player
	}
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
	current.SelectedCardIDs = nil
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
	player.SelectedCardIDs = nil
	player.SelectedSubmission = nil
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
	r.state = StateLobby
	r.round = 0
	r.czarIndex = -1
	r.currentBlackID = ""
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
		player.SelectedCardIDs = nil
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
	if r.currentBlackID != "" {
		r.blackDiscard = append(r.blackDiscard, r.currentBlackID)
	}
	for _, submission := range r.submissions {
		r.whiteDiscard = append(r.whiteDiscard, submission.CardIDs...)
	}
	for _, player := range r.players {
		player.Submitted = nil
	}
	r.submissions = nil
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
	r.currentBlackID = r.drawBlackLocked()
	black := r.currentBlackLocked()
	judge := r.judgeLocked()
	for _, player := range r.players {
		if player == judge {
			continue
		}
		for i := 0; i < black.Draw; i++ {
			player.Hand = append(player.Hand, r.drawWhiteLocked())
		}
	}
	r.round++
	r.state = StatePlaying
	if judge != nil {
		r.statusMessage = fmt.Sprintf("%s is the judge for round %d.", judge.Nickname, r.round)
	}
}

func (r *Room) drawWhiteLocked() string {
	if len(r.whiteDraw) == 0 {
		r.whiteDraw = append(r.whiteDraw, r.whiteDiscard...)
		r.whiteDiscard = nil
		r.rand.Shuffle(len(r.whiteDraw), func(i, j int) { r.whiteDraw[i], r.whiteDraw[j] = r.whiteDraw[j], r.whiteDraw[i] })
	}
	cardID := r.whiteDraw[len(r.whiteDraw)-1]
	r.whiteDraw = r.whiteDraw[:len(r.whiteDraw)-1]
	return cardID
}

func (r *Room) drawBlackLocked() string {
	if len(r.blackDraw) == 0 {
		r.blackDraw = append(r.blackDraw, r.blackDiscard...)
		r.blackDiscard = nil
		r.rand.Shuffle(len(r.blackDraw), func(i, j int) { r.blackDraw[i], r.blackDraw[j] = r.blackDraw[j], r.blackDraw[i] })
	}
	cardID := r.blackDraw[len(r.blackDraw)-1]
	r.blackDraw = r.blackDraw[:len(r.blackDraw)-1]
	return cardID
}

func (r *Room) judgeLocked() *Player {
	if len(r.players) == 0 || r.czarIndex < 0 || r.czarIndex >= len(r.players) {
		return nil
	}
	return r.players[r.czarIndex]
}

func (r *Room) currentBlackLocked() *deck.BlackCard {
	if r.currentBlackID == "" {
		return nil
	}
	return r.catalog.BlackCards[r.currentBlackID]
}

func (r *Room) playerLocked(player *Player) *Player {
	for _, current := range r.players {
		if current == player {
			return current
		}
	}
	return nil
}

func (r *Room) selectedDeckNamesLocked() []string {
	names := make([]string, 0, len(r.selectedDeckIDs))
	for _, id := range r.selectedDeckIDs {
		if d := r.catalog.DeckByID(id); d != nil {
			names = append(names, d.Name)
		}
	}
	return names
}

func (r *Room) SelectedDeckIDsForWhiteCard(cardID string) []string {
	if r == nil || cardID == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.selectedDeckIDsForCardLocked(cardID, false)
}

func (r *Room) SelectedDeckIDsForBlackCard(cardID string) []string {
	if r == nil || cardID == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.selectedDeckIDsForCardLocked(cardID, true)
}

func (r *Room) FirstSelectedDeckNameForWhiteCard(cardID string) string {
	if r == nil || cardID == "" {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.firstSelectedDeckNameForCardLocked(cardID, false)
}

func (r *Room) FirstSelectedDeckNameForBlackCard(cardID string) string {
	if r == nil || cardID == "" {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.firstSelectedDeckNameForCardLocked(cardID, true)
}

func (r *Room) selectedDeckIDsForCardLocked(cardID string, black bool) []string {
	deckIDs := make([]string, 0, len(r.selectedDeckIDs))
	for _, deckID := range r.selectedDeckIDs {
		d := r.catalog.DeckByID(deckID)
		if d == nil {
			continue
		}
		if black && slices.Contains(d.BlackIDs, cardID) {
			deckIDs = append(deckIDs, deckID)
			continue
		}
		if !black && slices.Contains(d.WhiteIDs, cardID) {
			deckIDs = append(deckIDs, deckID)
		}
	}
	return deckIDs
}

func (r *Room) firstSelectedDeckNameForCardLocked(cardID string, black bool) string {
	for _, deckID := range r.selectedDeckIDs {
		d := r.catalog.DeckByID(deckID)
		if d == nil {
			continue
		}
		if black && slices.Contains(d.BlackIDs, cardID) {
			return d.Name
		}
		if !black && slices.Contains(d.WhiteIDs, cardID) {
			return d.Name
		}
	}
	return ""
}

func (r *Room) lookupWhiteCardsLocked(ids []string) []*deck.WhiteCard {
	cards := make([]*deck.WhiteCard, 0, len(ids))
	for _, id := range ids {
		if card, ok := r.catalog.WhiteCards[id]; ok {
			cards = append(cards, card)
		}
	}
	return cards
}

func normalizeDeckIDs(catalog *deck.Catalog, ids []string) []string {
	if catalog == nil {
		return nil
	}
	if len(ids) == 0 {
		ids = catalog.DefaultDeckIDs()
	}
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if catalog.DeckByID(id) == nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

func normalizeIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func sortedSelected(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for id, enabled := range values {
		if enabled {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

func idsFromBlack(cards []*deck.BlackCard) []string {
	out := make([]string, len(cards))
	for i, card := range cards {
		out[i] = card.ID
	}
	return out
}

func idsFromWhite(cards []*deck.WhiteCard) []string {
	out := make([]string, len(cards))
	for i, card := range cards {
		out[i] = card.ID
	}
	return out
}

func submissionID(cardIDs []string) string {
	if len(cardIDs) == 0 {
		return ""
	}
	return strings.Join(cardIDs, "+")
}

func (r *Room) setTargetScoreLocked(player *Player, score int) error {
	if score < 2 {
		score = 2
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func randomCode() (string, error) {
	var raw [roomCodeLength]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, n := range raw {
		b.WriteByte(roomCodeAlphabet[n&31])
	}
	return b.String(), nil
}

func newCryptoRand() (*mathrand.Rand, error) {
	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, err
	}
	return mathrand.New(mathrand.NewSource(int64(binary.LittleEndian.Uint64(seed[:])))), nil
}

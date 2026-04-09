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
	ErrNeedNickname        = errors.New("nickname is required")
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

type Options struct {
	MinPlayers int
}

type RoomState string

const (
	StateLobby   RoomState = "lobby"
	StatePlaying RoomState = "playing"
	StateJudging RoomState = "judging"
)

type Manager struct {
	mu      sync.RWMutex
	rooms   map[string]*Room
	catalog *deck.Catalog
	opts    Options
}

type Room struct {
	code            string
	catalog         *deck.Catalog
	rand            *mathrand.Rand
	minPlayers      int
	mu              sync.RWMutex
	hostID          string
	players         []*Player
	selectedDeckIDs []string
	state           RoomState
	round           int
	czarIndex       int
	currentBlackID  string
	lastWinnerName  string
	statusMessage   string
	blackDraw       []string
	blackDiscard    []string
	whiteDraw       []string
	whiteDiscard    []string
	submissions     []*Submission
}

type Player struct {
	ID        string
	Nickname  string
	Score     int
	Hand      []string
	Submitted []string
}

type Submission struct {
	ID       string
	PlayerID string
	CardIDs  []string
}

type RoomSummary struct {
	Code      string
	HostName  string
	State     RoomState
	Players   int
	DeckNames []string
}

type PlayerView struct {
	ID        string
	Nickname  string
	Score     int
	IsHost    bool
	IsJudge   bool
	Submitted bool
}

type SubmissionView struct {
	ID         string
	Submission *Submission
	Cards      []*deck.WhiteCard
}

type RoomView struct {
	Exists          bool
	Code            string
	State           RoomState
	HostName        string
	Players         []PlayerView
	SelectedDeckIDs []string
	SelectedDecks   []string
	BlackCount      int
	WhiteCount      int
	RequiredWhite   int
	CanJoin         bool
	CanStart        bool
	IsHost          bool
	InRoom          bool
	CanSubmit       bool
	CanJudge        bool
	NeedPick        int
	NeedDraw        int
	CurrentBlack    *deck.BlackCard
	Hand            []*deck.WhiteCard
	Submissions     []SubmissionView
	LastWinnerName  string
	StatusMessage   string
	JudgeName       string
}

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

func (m *Manager) CreateRoom(playerID, nickname string, defaultDeckIDs []string) (*Room, error) {
	if strings.TrimSpace(nickname) == "" {
		return nil, ErrNeedNickname
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, room := range m.rooms {
		if room.hasPlayerLocked(playerID) {
			return nil, ErrAlreadyInRoom
		}
	}
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
		hostID:          playerID,
		state:           StateLobby,
		czarIndex:       -1,
		selectedDeckIDs: normalizeDeckIDs(m.catalog, defaultDeckIDs),
		players: []*Player{{
			ID:       playerID,
			Nickname: strings.TrimSpace(nickname),
		}},
	}
	m.rooms[code] = room
	return room, nil
}

func (m *Manager) GetRoom(code string) *Room {
	m.mu.RLock()
	room := m.rooms[strings.ToUpper(strings.TrimSpace(code))]
	m.mu.RUnlock()
	return room
}

func (m *Manager) JoinRoom(code, playerID, nickname string) (*Room, error) {
	if strings.TrimSpace(nickname) == "" {
		return nil, ErrNeedNickname
	}
	m.mu.RLock()
	room := m.rooms[strings.ToUpper(strings.TrimSpace(code))]
	var alreadyInRoom bool
	if room != nil {
		room.mu.RLock()
		alreadyInRoom = room.hasPlayerLocked(playerID)
		room.mu.RUnlock()
	}
	m.mu.RUnlock()
	if room == nil {
		return nil, ErrRoomNotFound
	}
	if alreadyInRoom {
		return room, nil
	}
	m.mu.RLock()
	for _, other := range m.rooms {
		other.mu.RLock()
		inOther := other.hasPlayerLocked(playerID)
		other.mu.RUnlock()
		if inOther {
			m.mu.RUnlock()
			return nil, ErrAlreadyInRoom
		}
	}
	m.mu.RUnlock()
	if err := room.join(playerID, nickname); err != nil {
		return nil, err
	}
	return room, nil
}

func (m *Manager) LeaveRoom(code, playerID string) (*Room, bool) {
	m.mu.RLock()
	room := m.rooms[strings.ToUpper(strings.TrimSpace(code))]
	m.mu.RUnlock()
	if room == nil {
		return nil, false
	}
	destroy := room.leave(playerID)
	if destroy {
		m.mu.Lock()
		if m.rooms[room.code] == room {
			delete(m.rooms, room.code)
		}
		m.mu.Unlock()
	}
	return room, destroy
}

func (m *Manager) ReconcileMembership(code, playerID string) bool {
	room := m.GetRoom(code)
	if room == nil {
		return false
	}
	room.mu.RLock()
	ok := room.hasPlayerLocked(playerID)
	room.mu.RUnlock()
	return ok
}

func (m *Manager) RoomSummaries() []RoomSummary {
	m.mu.RLock()
	rooms := make([]*Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		rooms = append(rooms, room)
	}
	m.mu.RUnlock()
	slices.SortFunc(rooms, func(a, b *Room) int { return strings.Compare(a.code, b.code) })
	out := make([]RoomSummary, 0, len(rooms))
	for _, room := range rooms {
		out = append(out, room.summary())
	}
	return out
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

func (r *Room) ToggleDeck(playerID, deckID string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if playerID != r.hostID {
		return ErrOnlyHostCanEdit
	}
	if r.state != StateLobby {
		return ErrDecksLocked
	}
	if r.catalog.DeckByID(deckID) == nil {
		return ErrUnknownDeck
	}
	selected := make(map[string]bool, len(r.selectedDeckIDs))
	for _, id := range r.selectedDeckIDs {
		selected[id] = true
	}
	if enabled {
		selected[deckID] = true
	} else {
		delete(selected, deckID)
	}
	r.selectedDeckIDs = sortedSelected(selected)
	return nil
}

func (r *Room) IsDeckSelected(deckID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Contains(r.selectedDeckIDs, deckID)
}

func (r *Room) Start(playerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if playerID != r.hostID {
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
	r.statusMessage = ""
	r.round = 0
	r.czarIndex = -1
	for _, player := range r.players {
		player.Score = 0
		player.Hand = nil
		player.Submitted = nil
	}
	r.advanceRoundLocked()
	return nil
}

func (r *Room) PlayCards(playerID string, cardIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	player := r.playerLocked(playerID)
	if player == nil {
		return ErrOnlyPlayersCanPlay
	}
	if r.state != StatePlaying {
		return ErrNotYourTurn
	}
	if r.playerIsJudgeLocked(playerID) {
		return ErrJudgeCannotPlay
	}
	if len(player.Submitted) > 0 {
		return ErrAlreadySubmitted
	}
	cardIDs = normalizeIDs(cardIDs)
	if len(cardIDs) != r.currentBlackLocked().Pick {
		return ErrNeedExactCards
	}
	handSet := make(map[string]int, len(player.Hand))
	for i, id := range player.Hand {
		handSet[id] = i
	}
	for _, cardID := range cardIDs {
		if _, ok := handSet[cardID]; !ok {
			return ErrCardNotInHand
		}
	}
	remaining := make([]string, 0, len(player.Hand)-len(cardIDs))
	selected := make(map[string]struct{}, len(cardIDs))
	for _, cardID := range cardIDs {
		selected[cardID] = struct{}{}
	}
	for _, cardID := range player.Hand {
		if _, ok := selected[cardID]; ok {
			continue
		}
		remaining = append(remaining, cardID)
	}
	player.Hand = remaining
	player.Submitted = append([]string(nil), cardIDs...)
	r.submissions = append(r.submissions, &Submission{
		ID:       cardIDs[0],
		PlayerID: playerID,
		CardIDs:  append([]string(nil), cardIDs...),
	})
	if len(r.submissions) == len(r.players)-1 {
		r.rand.Shuffle(len(r.submissions), func(i, j int) {
			r.submissions[i], r.submissions[j] = r.submissions[j], r.submissions[i]
		})
		r.state = StateJudging
		r.statusMessage = fmt.Sprintf("%s is judging the round.", r.judgeLocked().Nickname)
	}
	return nil
}

func (r *Room) Judge(playerID, submissionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != StateJudging {
		return ErrNotYourTurn
	}
	if !r.playerIsJudgeLocked(playerID) {
		return ErrNotJudge
	}
	var winner *Player
	for _, submission := range r.submissions {
		if submission.ID == submissionID {
			winner = r.playerLocked(submission.PlayerID)
			break
		}
	}
	if winner == nil {
		return ErrSubmissionNotFound
	}
	winner.Score++
	r.lastWinnerName = winner.Nickname
	if winner.Score >= ScoreGoal {
		r.resetToLobbyLocked(fmt.Sprintf("%s won the game. Room reset to the lobby.", winner.Nickname))
		return nil
	}
	r.statusMessage = fmt.Sprintf("%s won the round.", winner.Nickname)
	r.advanceRoundLocked()
	return nil
}

func (r *Room) Snapshot(playerID string) RoomView {
	r.mu.RLock()
	defer r.mu.RUnlock()
	view := RoomView{
		Exists:          true,
		Code:            r.code,
		State:           r.state,
		SelectedDeckIDs: append([]string(nil), r.selectedDeckIDs...),
		LastWinnerName:  r.lastWinnerName,
		StatusMessage:   r.statusMessage,
	}
	blackCount, whiteCount, _ := r.catalog.UnionCounts(r.selectedDeckIDs)
	view.BlackCount = blackCount
	view.WhiteCount = whiteCount
	view.RequiredWhite = MinWhiteCardsPerPlayer * max(len(r.players), 1)
	view.SelectedDecks = r.selectedDeckNamesLocked()
	player := r.playerLocked(playerID)
	view.InRoom = player != nil
	view.IsHost = playerID != "" && playerID == r.hostID
	view.CanJoin = player == nil && r.state == StateLobby && len(r.players) < MaxPlayers
	view.CanStart = view.IsHost && r.state == StateLobby && len(r.players) >= r.minPlayers && blackCount >= MinBlackCards && whiteCount >= MinWhiteCardsPerPlayer*len(r.players)
	if host := r.playerLocked(r.hostID); host != nil {
		view.HostName = host.Nickname
	}
	var judgeID string
	if judge := r.judgeLocked(); judge != nil {
		judgeID = judge.ID
		view.JudgeName = judge.Nickname
	}
	for _, p := range r.players {
		view.Players = append(view.Players, PlayerView{
			ID:        p.ID,
			Nickname:  p.Nickname,
			Score:     p.Score,
			IsHost:    p.ID == r.hostID,
			IsJudge:   p.ID == judgeID && r.state != StateLobby,
			Submitted: len(p.Submitted) > 0,
		})
	}
	if black := r.currentBlackLocked(); black != nil {
		view.CurrentBlack = black
		view.NeedPick = black.Pick
		view.NeedDraw = black.Draw
	}
	if player != nil {
		view.Hand = r.lookupWhiteCardsLocked(player.Hand)
		view.CanSubmit = r.state == StatePlaying && !r.playerIsJudgeLocked(playerID) && len(player.Submitted) == 0
		view.CanJudge = r.state == StateJudging && r.playerIsJudgeLocked(playerID)
	}
	if r.state == StateJudging {
		for _, submission := range r.submissions {
			view.Submissions = append(view.Submissions, SubmissionView{
				ID:         submission.ID,
				Submission: submission,
				Cards:      r.lookupWhiteCardsLocked(submission.CardIDs),
			})
		}
	}
	return view
}

func (r *Room) summary() RoomSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()
	summary := RoomSummary{
		Code:      r.code,
		State:     r.state,
		Players:   len(r.players),
		DeckNames: r.selectedDeckNamesLocked(),
	}
	if host := r.playerLocked(r.hostID); host != nil {
		summary.HostName = host.Nickname
	}
	return summary
}

func (r *Room) join(playerID, nickname string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hasPlayerLocked(playerID) {
		return nil
	}
	if len(r.players) >= MaxPlayers {
		return ErrRoomFull
	}
	if r.state != StateLobby {
		return ErrGameInProgress
	}
	r.players = append(r.players, &Player{
		ID:       playerID,
		Nickname: strings.TrimSpace(nickname),
	})
	if r.hostID == "" {
		r.hostID = playerID
	}
	r.statusMessage = fmt.Sprintf("%s joined the room.", nickname)
	return nil
}

func (r *Room) leave(playerID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	player := r.playerLocked(playerID)
	if player == nil {
		return len(r.players) == 0
	}
	wasJudge := r.playerIsJudgeLocked(playerID)
	idx := slices.IndexFunc(r.players, func(p *Player) bool { return p.ID == playerID })
	if idx < 0 {
		return len(r.players) == 0
	}
	if idx < r.czarIndex {
		r.czarIndex--
	}
	r.whiteDiscard = append(r.whiteDiscard, player.Hand...)
	r.whiteDiscard = append(r.whiteDiscard, player.Submitted...)
	r.players = append(r.players[:idx], r.players[idx+1:]...)
	r.submissions = slices.DeleteFunc(r.submissions, func(sub *Submission) bool { return sub.PlayerID == playerID })
	if r.hostID == playerID {
		if len(r.players) > 0 {
			r.hostID = r.players[0].ID
		} else {
			r.hostID = ""
		}
	}
	if len(r.players) == 0 {
		return true
	}
	if r.state != StateLobby {
		if len(r.players) < r.minPlayers {
			r.resetToLobbyLocked("Not enough players to continue. Room reset to the lobby.")
		} else if wasJudge {
			r.resetToLobbyLocked("The judge left. Room reset to the lobby.")
		} else {
			if len(r.submissions) == len(r.players)-1 && r.state == StatePlaying {
				r.rand.Shuffle(len(r.submissions), func(i, j int) { r.submissions[i], r.submissions[j] = r.submissions[j], r.submissions[i] })
				r.state = StateJudging
			}
			r.statusMessage = fmt.Sprintf("%s left the room.", player.Nickname)
		}
	} else {
		r.statusMessage = fmt.Sprintf("%s left the room.", player.Nickname)
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
	}
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
	for _, player := range r.players {
		if player.ID == r.judgeLocked().ID {
			continue
		}
		for i := 0; i < black.Draw; i++ {
			player.Hand = append(player.Hand, r.drawWhiteLocked())
		}
	}
	r.round++
	r.state = StatePlaying
	r.statusMessage = fmt.Sprintf("%s is the judge for round %d.", r.judgeLocked().Nickname, r.round)
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

func (r *Room) playerLocked(playerID string) *Player {
	for _, player := range r.players {
		if player.ID == playerID {
			return player
		}
	}
	return nil
}

func (r *Room) playerIsJudgeLocked(playerID string) bool {
	judge := r.judgeLocked()
	return judge != nil && judge.ID == playerID
}

func (r *Room) hasPlayerLocked(playerID string) bool {
	return r.playerLocked(playerID) != nil
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

package game

import (
	mathrand "math/rand"
	"sync"
	"time"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/deck"
)

type Options struct {
	MinPlayers int
	Debug      bool
}

type RoomState string

const (
	StateLobby   RoomState = "lobby"
	StatePlaying RoomState = "playing"
	StateJudging RoomState = "judging"
	StateReview  RoomState = "results"
)

type Manager struct {
	mu      sync.RWMutex
	rooms   map[string]*Room
	catalog *deck.Catalog
	opts    Options
	dirty   func(...any)
}

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

type Player struct {
	Session *jaws.Session

	Nickname           string
	NicknameInput      string
	Room               *Room
	Score              int
	Hand               []*deck.WhiteCard
	Submitted          []*deck.WhiteCard
	SelectedCards      []*deck.WhiteCard
	SelectedSubmission *Submission

	uiMu sync.Mutex
}

type Submission struct {
	ID     string
	Player *Player
	Cards  []*deck.WhiteCard
}

type FinalScore struct {
	Player   *Player
	Nickname string
	Score    int
	IsWinner bool
}

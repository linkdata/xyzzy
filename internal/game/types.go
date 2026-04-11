package game

import (
	mathrand "math/rand"
	"sync"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/deck"
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
	host            *Player
	players         []*Player
	selectedDeckIDs []string
	targetScore     int
	state           RoomState
	round           int
	czarIndex       int
	currentBlackID  string
	lastWinnerName  string
	lastGameWinner  string
	lastGameScores  []FinalScore
	statusMessage   string
	blackDraw       []string
	blackDiscard    []string
	whiteDraw       []string
	whiteDiscard    []string
	submissions     []*Submission
}

type Player struct {
	Session *jaws.Session

	Nickname           string
	NicknameInput      string
	Room               *Room
	Score              int
	Hand               []string
	Submitted          []string
	SelectedCardIDs    []string
	SelectedSubmission *Submission

	uiMu sync.Mutex
}

type Submission struct {
	ID      string
	Player  *Player
	CardIDs []string
}

type FinalScore struct {
	Player   *Player
	Nickname string
	Score    int
	IsWinner bool
}

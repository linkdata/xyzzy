package game

import (
	mathrand "math/rand"
	"sync"

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
	hostID          string
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

type FinalScore struct {
	PlayerID string
	Nickname string
	Score    int
	IsWinner bool
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
	LastGameWinner  string
	LastGameScores  []FinalScore
	LastWinnerName  string
	StatusMessage   string
	JudgeName       string
	TargetScore     int
}

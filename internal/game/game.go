package game

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	mathrand "math/rand"
	"slices"
	"strings"
	"time"

	"github.com/linkdata/xyzzy/internal/deck"
)

const (
	// MinPlayers is the default number of players required to start a game.
	MinPlayers = 3
	// MaxPlayers is the maximum number of players allowed in a room.
	MaxPlayers = 10
	// HandSize is the number of white cards normally held by each player.
	HandSize = 10
	// ScoreGoal is the default number of points required to win a game.
	ScoreGoal = 5
	// ReviewDelay is the automatic delay between a result and the next state.
	ReviewDelay = 30 * time.Second
	// MinBlackCards is the minimum selected black-card count required to start.
	MinBlackCards = 50
	// MinWhiteCardsPerPlayer is the selected white-card requirement per player.
	MinWhiteCardsPerPlayer = 20
	roomCodeLength         = 6
	roomCodeAlphabet       = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

var (
	// ErrRoomNotFound indicates that a requested room does not exist.
	ErrRoomNotFound = errors.New("room not found")
	// ErrRoomFull indicates that a room has reached [MaxPlayers].
	ErrRoomFull = errors.New("room is full")
	// ErrGameInProgress indicates that an operation requires a lobby room.
	ErrGameInProgress = errors.New("game already in progress")
	// ErrAlreadyInRoom indicates that a player is already seated in a room.
	ErrAlreadyInRoom = errors.New("player is already in a room")
	// ErrOnlyHostCanEdit indicates that a non-host tried to edit lobby settings.
	ErrOnlyHostCanEdit = errors.New("only the host can change lobby settings")
	// ErrDecksLocked indicates that a deck edit was attempted outside the lobby.
	ErrDecksLocked = errors.New("deck selection is locked after the game starts")
	// ErrUnknownDeck indicates that a deck is not from the manager's catalog.
	ErrUnknownDeck = errors.New("unknown deck")
	// ErrNotEnoughBlackCards indicates that the selected decks cannot start a game.
	ErrNotEnoughBlackCards = errors.New("selected decks need at least 50 unique black cards")
	// ErrNotEnoughWhiteCards indicates that the selected decks cannot seat or deal all players.
	ErrNotEnoughWhiteCards = errors.New("selected decks need at least 20 white cards per player")
	// ErrOnlyHostCanStart indicates that a non-host tried to start the game.
	ErrOnlyHostCanStart = errors.New("only the host can start the game")
	// ErrOnlyPlayersCanPlay indicates that an unseated player tried to submit cards.
	ErrOnlyPlayersCanPlay = errors.New("only players in the room can play")
	// ErrNotYourTurn indicates that an action is invalid in the current room state.
	ErrNotYourTurn = errors.New("not your turn")
	// ErrJudgeCannotPlay indicates that the current judge tried to submit cards.
	ErrJudgeCannotPlay = errors.New("judge cannot play cards")
	// ErrNeedExactCards indicates that a submission has the wrong number of unique cards.
	ErrNeedExactCards = errors.New("must select the exact number of cards")
	// ErrCardNotInHand indicates that a submitted card is absent from the player's hand.
	ErrCardNotInHand = errors.New("selected card is not in your hand")
	// ErrAlreadySubmitted indicates that a player has already submitted this round.
	ErrAlreadySubmitted = errors.New("cards already submitted")
	// ErrSubmissionNotFound indicates that a submission is not in the current round.
	ErrSubmissionNotFound = errors.New("submission not found")
	// ErrNotJudge indicates that a judge-only action was attempted by another player.
	ErrNotJudge = errors.New("only the judge can pick a winner")
	// ErrReviewNotReady indicates that review progression was attempted outside review.
	ErrReviewNotReady = errors.New("round result is not ready")
)

// NewManager creates a manager with default options.
func NewManager(catalog *deck.Catalog) (result *Manager) {
	result = NewManagerWithOptions(catalog, Options{})
	return
}

// NewManagerWithOptions creates a manager using catalog and a copy of opts.
//
// An [Options.MinPlayers] value below two uses [MinPlayers]. The manager retains
// catalog, whose contents must remain immutable.
func NewManagerWithOptions(catalog *deck.Catalog, opts Options) (result *Manager) {
	if opts.MinPlayers < 2 {
		opts.MinPlayers = MinPlayers
	}
	result = &Manager{
		rooms:   make(map[string]*Room),
		catalog: catalog,
		opts:    opts,
	}
	return
}

// NormalizeNickname removes characters other than ASCII letters and digits.
//
// It returns "Player" when no accepted characters remain.
func NormalizeNickname(raw string) (result string) {
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
		result = "Player"
		return
	}
	result = b.String()
	return
}

func normalizeDecks(catalog *deck.Catalog, decks []*deck.Deck) (result []*deck.Deck) {
	if catalog == nil {
		return
	}
	if len(decks) == 0 {
		decks = catalog.DefaultDecks()
	}
	result = make([]*deck.Deck, 0, len(decks))
	seen := make(map[*deck.Deck]struct{}, len(decks))
	for _, selected := range decks {
		if selected == nil {
			continue
		}
		if canonical := catalog.DeckByID(selected.ID); canonical != selected {
			continue
		}
		if _, ok := seen[selected]; ok {
			continue
		}
		seen[selected] = struct{}{}
		result = append(result, selected)
	}
	slices.SortFunc(result, func(a, b *deck.Deck) (cmp int) { cmp = strings.Compare(a.ID, b.ID); return })
	return
}

func normalizeWhiteCards(cards []*deck.WhiteCard) (result []*deck.WhiteCard) {
	seen := make(map[*deck.WhiteCard]struct{}, len(cards))
	result = make([]*deck.WhiteCard, 0, len(cards))
	for _, card := range cards {
		if card == nil {
			continue
		}
		if _, ok := seen[card]; ok {
			continue
		}
		seen[card] = struct{}{}
		result = append(result, card)
	}
	return
}

func sortedSelectedDecks(values map[*deck.Deck]bool) (result []*deck.Deck) {
	result = make([]*deck.Deck, 0, len(values))
	for selected, enabled := range values {
		if enabled {
			result = append(result, selected)
		}
	}
	slices.SortFunc(result, func(a, b *deck.Deck) (cmp int) { cmp = strings.Compare(a.ID, b.ID); return })
	return
}

func submissionID(round, seq int) (result string) {
	result = fmt.Sprintf("r%d-s%d", round, seq)
	return
}

func randomCode() (result string) {
	var raw [roomCodeLength]byte
	_, _ = rand.Read(raw[:])
	var b strings.Builder
	for _, n := range raw {
		b.WriteByte(roomCodeAlphabet[n&31])
	}
	return b.String()
}

func newCryptoRand() *mathrand.Rand {
	var seed [8]byte
	_, _ = rand.Read(seed[:])
	return mathrand.New(mathrand.NewSource(int64(binary.LittleEndian.Uint64(seed[:]))))
}

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
	MinPlayers             = 3
	MaxPlayers             = 10
	HandSize               = 10
	ScoreGoal              = 5
	ReviewDelay            = 30 * time.Second
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
	ErrOnlyHostCanEdit     = errors.New("only the host can change lobby settings")
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
	ErrReviewNotReady      = errors.New("round result is not ready")
)

func NewManager(catalog *deck.Catalog) (result *Manager) {
	result = NewManagerWithOptions(catalog, Options{})
	return
}

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

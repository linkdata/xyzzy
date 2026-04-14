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

func normalizeDeckIDs(catalog *deck.Catalog, ids []string) (result []string) {
	if catalog == nil {
		result = nil
		return
	}
	if len(ids) == 0 {
		ids = catalog.DefaultDeckIDs()
	}
	result = make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if catalog.DeckByID(id) == nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	slices.Sort(result)
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

func sortedSelected(values map[string]bool) (result []string) {
	result = make([]string, 0, len(values))
	for id, enabled := range values {
		if enabled {
			result = append(result, id)
		}
	}
	slices.Sort(result)
	return
}

func submissionID(round, seq int) (result string) {
	result = fmt.Sprintf("r%d-s%d", round, seq)
	return
}

func randomCode() (result1 string, errResult error) {
	var raw [roomCodeLength]byte
	if _, err := rand.Read(raw[:]); err != nil {
		result1, errResult = "", err
		return
	}
	var b strings.Builder
	for _, n := range raw {
		b.WriteByte(roomCodeAlphabet[n&31])
	}
	result1, errResult = b.String(), nil
	return
}

func newCryptoRand() (result1 *mathrand.Rand, errResult error) {
	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		result1, errResult = nil, err
		return
	}
	result1, errResult = mathrand.New(mathrand.NewSource(int64(binary.LittleEndian.Uint64(seed[:])))), nil
	return
}

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

func normalizeWhiteCards(cards []*deck.WhiteCard) []*deck.WhiteCard {
	seen := make(map[*deck.WhiteCard]struct{}, len(cards))
	out := make([]*deck.WhiteCard, 0, len(cards))
	for _, card := range cards {
		if card == nil {
			continue
		}
		if _, ok := seen[card]; ok {
			continue
		}
		seen[card] = struct{}{}
		out = append(out, card)
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

func submissionID(round, seq int) string {
	return fmt.Sprintf("r%d-s%d", round, seq)
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

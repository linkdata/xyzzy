package game

import (
	"sync"
	"sync/atomic"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/deck"
)

// Player represents one participant's identity and game state.
//
// The zero value is detached. Room membership changes are published atomically
// and observed with [Player.Room]. Once a player is shared between goroutines,
// callers must use the concurrency-safe [Player], [Manager], and [Room] methods
// documented for each field instead of accessing mutable fields directly.
type Player struct {
	// Session identifies the player's JaWS session. It is initialized before
	// publication and immutable afterward. It may be nil when session expiry is
	// not managed; [Manager.CleanupExpiredSessions] treats nil as expired.
	Session *jaws.Session

	// Nickname is the room-visible name. Use [Manager.SetNickname] and
	// [Player.NicknameValue] during concurrent use.
	Nickname string
	// NicknameInput is the editable nickname value. Use [Player.NicknameField]
	// and [Player.NicknameInputValue] during concurrent use.
	NicknameInput string
	// Score is managed by the player's current [Room]. Use [Room.ScoreFor]
	// during concurrent use.
	Score int
	// Hand is managed by the player's current [Room]. Use [Room.HandFor] during
	// concurrent use.
	Hand []*deck.WhiteCard
	// Submitted is managed by the player's current [Room]. Use [Room.SubmittedBy]
	// during concurrent use.
	Submitted []*deck.WhiteCard
	// SelectedCards is managed by [Room.ToggleCardSelection]. Use
	// [Room.SelectionOrderFor] during concurrent use.
	SelectedCards []*deck.WhiteCard
	// SelectedSubmission is managed by [Room.ToggleSubmissionSelection]. Use
	// [Room.SubmissionSelected] during concurrent use.
	SelectedSubmission *Submission

	room atomic.Pointer[Room]
	// Nested locking follows Manager.mu -> Room.mu -> Player.uiMu.
	uiMu sync.Mutex
}

// Room returns the room in which the player is currently seated.
//
// It returns nil for a nil or detached player and is safe to call concurrently.
func (p *Player) Room() (result *Room) {
	if p != nil {
		result = p.room.Load()
	}
	return
}

// setRoom updates the player's current room. Writes are expected to happen
// while the manager and room locks are held; the atomic store ensures
// concurrent readers see a consistent value.
func (p *Player) setRoom(r *Room) {
	p.room.Store(r)
}

// NicknameValue returns the player's room-visible nickname.
//
// It returns an empty string for a nil player and is safe to call concurrently.
func (p *Player) NicknameValue() (result string) {
	if p != nil {
		p.uiMu.Lock()
		result = p.Nickname
		p.uiMu.Unlock()
	}
	return
}

// NicknameInputValue returns the player's current nickname input.
//
// It returns an empty string for a nil player and is safe to call concurrently.
func (p *Player) NicknameInputValue() (result string) {
	if p != nil {
		p.uiMu.Lock()
		result = p.NicknameInput
		p.uiMu.Unlock()
	}
	return
}

func (p *Player) setNickname(nickname string) (changed bool) {
	if p != nil {
		p.uiMu.Lock()
		if changed = p.Nickname != nickname || p.NicknameInput != nickname; changed {
			p.Nickname = nickname
			p.NicknameInput = nickname
		}
		p.uiMu.Unlock()
	}
	return
}

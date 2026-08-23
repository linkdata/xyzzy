package game

import (
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/linkdata/xyzzy/internal/deck"
)

// Manager owns the active rooms and coordinates room membership.
//
// Its methods are safe for concurrent use. Managers must be constructed with
// [NewManager] or [NewManagerWithOptions]; the zero value is not ready for use.
type Manager struct {
	mu      sync.RWMutex
	rooms   map[string]*Room
	catalog *deck.Catalog
	opts    Options
}

func (m *Manager) notify(tags ...any) {
	if dirty := m.opts.Dirty; dirty != nil {
		dirty(tags...)
	}
}

// CreateRoom creates a room and seats player as its host.
//
// An empty defaultDecks slice uses the catalog defaults. It returns
// [ErrAlreadyInRoom] when player is nil or already seated.
func (m *Manager) CreateRoom(player *Player, defaultDecks []*deck.Deck) (room *Room, err error) {
	err = ErrAlreadyInRoom
	if player != nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		if player.Room() == nil {
			var code string
			if code, err = m.newRoomCodeLocked(); err == nil {
				room = &Room{
					manager:       m,
					code:          code,
					catalog:       m.catalog,
					rand:          newRoomRand(),
					minPlayers:    m.opts.MinPlayers,
					debug:         m.opts.Debug,
					reviewDelay:   ReviewDelay,
					targetScore:   ScoreGoal,
					state:         StateLobby,
					czarIndex:     -1,
					selectedDecks: normalizeDecks(m.catalog, defaultDecks),
				}
				room.seatLocked(player)
				room.host = player
				room.players = []*Player{player}
				m.rooms[code] = room
			}
		}
	}
	return
}

// Room returns the room matching code, or nil if no room matches.
func (m *Manager) Room(code string) (result *Room) {
	m.mu.RLock()
	result = m.rooms[strings.ToUpper(strings.TrimSpace(code))]
	m.mu.RUnlock()
	return
}

// Rooms returns all active rooms ordered by code.
//
// The returned slice has independent storage.
func (m *Manager) Rooms() (result []*Room) {
	m.mu.RLock()
	result = make([]*Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		result = append(result, room)
	}
	m.mu.RUnlock()
	slices.SortFunc(result, func(a, b *Room) (result int) { return strings.Compare(a.code, b.code) })
	return
}

// PublicRooms returns the non-private active rooms ordered by code.
//
// The returned slice has independent storage.
func (m *Manager) PublicRooms() (result []*Room) {
	for _, room := range m.Rooms() {
		if !room.IsPrivate() {
			result = append(result, room)
		}
	}
	return
}

// SetNickname normalizes and stores the player's nickname.
//
// The operation is serialized with room membership changes. A seated nickname
// is unique within its [Room]. Changed dependency tags are passed to the
// configured Dirty callback in [Options]. A nil player or unchanged value does
// not invoke the callback.
func (m *Manager) SetNickname(player *Player, nickname string) {
	if player != nil {
		nickname = NormalizeNickname(nickname)
		var room *Room
		var changed bool
		m.mu.RLock()
		if room = player.Room(); room != nil {
			changed = room.setNickname(player, nickname)
		} else {
			changed = player.setNickname(nickname)
		}
		m.mu.RUnlock()

		if !changed {
			return
		}
		if room != nil {
			m.notify(m, player, &player.NicknameInput, room)
			return
		}
		m.notify(m, player, &player.NicknameInput)
	}
}

// JoinRoom seats player in the room matching code.
//
// Room lookup and seating are one operation, so a concurrent leave cannot
// remove the room between them.
func (m *Manager) JoinRoom(code string, player *Player) (room *Room, err error) {
	err = ErrRoomNotFound
	if player != nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		room = m.rooms[strings.ToUpper(strings.TrimSpace(code))]
		if room != nil {
			err = ErrAlreadyInRoom
			if player.Room() != room {
				err = room.join(player)
			}
		}
	}
	return
}

// LeaveRoom removes player from their current room.
//
// If the room becomes empty, LeaveRoom also removes it from the manager.
func (m *Manager) LeaveRoom(player *Player) (room *Room, empty bool) {
	if player != nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		if room = player.Room(); room != nil {
			if empty = room.leave(player); empty && m.rooms[room.code] == room {
				delete(m.rooms, room.code)
			}
		}
	}
	return
}

// CleanupExpiredSessions removes players whose JaWS session is nil or expired.
//
// It deletes rooms that become empty and returns every affected room, including
// deleted rooms, ordered by code. The returned slice has independent storage.
func (m *Manager) CleanupExpiredSessions() (result []*Room) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result = make([]*Room, 0)
	for code, room := range m.rooms {
		expired := room.expiredPlayers()
		if len(expired) == 0 {
			continue
		}
		result = append(result, room)
		for _, player := range expired {
			if room.leave(player) {
				delete(m.rooms, code)
				break
			}
		}
	}
	slices.SortFunc(result, func(a, b *Room) (cmp int) { cmp = strings.Compare(a.code, b.code); return })
	return
}

func (m *Manager) newRoomCodeLocked() (roomCode string, err error) {
	for range 1024 {
		s := randomCode()
		if _, exists := m.rooms[s]; !exists {
			roomCode = s
			return
		}
	}
	err = errors.New("could not allocate room code")
	return
}

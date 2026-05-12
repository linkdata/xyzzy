package game

import (
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/linkdata/xyzzy/internal/deck"
)

type Manager struct {
	mu      sync.RWMutex
	rooms   map[string]*Room
	catalog *deck.Catalog
	opts    Options
	dirtyMu sync.RWMutex
	dirty   func(...any)
}

func (m *Manager) SetDirty(fn func(...any)) {
	m.dirtyMu.Lock()
	m.dirty = fn
	m.dirtyMu.Unlock()
}

func (m *Manager) notify(tags ...any) {
	m.dirtyMu.RLock()
	dirty := m.dirty
	m.dirtyMu.RUnlock()
	if dirty != nil {
		dirty(tags...)
	}
}

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
					rand:          newCryptoRand(),
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

func (m *Manager) Room(code string) (result *Room) {
	m.mu.RLock()
	result = m.rooms[strings.ToUpper(strings.TrimSpace(code))]
	m.mu.RUnlock()
	return
}

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

func (m *Manager) PublicRooms() (result []*Room) {
	for _, room := range m.Rooms() {
		if !room.IsPrivate() {
			result = append(result, room)
		}
	}
	return
}

// JoinRoom seats the player in the room with the given code. The manager
// lock is held for the whole operation so the room cannot be removed from
// the registry by a concurrent leave between lookup and join.
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

// LeaveRoom removes the player from their current room. If that empties
// the room it is also removed from the registry while the manager lock is
// still held, so no concurrent JoinRoom can seat a player into a room
// that is about to disappear.
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

// CleanupExpiredSessions drops players whose JaWS sessions have expired
// and deletes any rooms that empty out as a result. Returns the rooms
// that were affected (including ones that were deleted).
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
	for i := 0; i < 1024; i++ {
		s := randomCode()
		if _, exists := m.rooms[s]; !exists {
			roomCode = s
			return
		}
	}
	err = errors.New("could not allocate room code")
	return
}

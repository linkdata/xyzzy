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
	dirty   func(...any)
}

func (m *Manager) SetDirty(fn func(...any)) {
	m.mu.Lock()
	m.dirty = fn
	m.mu.Unlock()
}

func (m *Manager) notify(tags ...any) {
	m.mu.RLock()
	dirty := m.dirty
	m.mu.RUnlock()
	if dirty != nil {
		dirty(tags...)
	}
}

func (m *Manager) CreateRoom(player *Player, defaultDecks []*deck.Deck) (room *Room, err error) {
	err = ErrAlreadyInRoom
	if player != nil && player.Room == nil {
		m.mu.Lock()
		defer m.mu.Unlock()
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

func (m *Manager) JoinRoom(code string, player *Player) (room *Room, err error) {
	err = ErrRoomNotFound
	if player != nil {
		room = m.Room(code)
		if room != nil {
			err = ErrAlreadyInRoom
			if player.Room != room {
				err = room.join(player)
			}
		}
	}
	return
}

func (m *Manager) LeaveRoom(player *Player) (room *Room, empty bool) {
	if player != nil && player.Room != nil {
		room = player.Room
		if empty = room.leave(player); empty {
			m.mu.Lock()
			if m.rooms[room.code] == room {
				delete(m.rooms, room.code)
			}
			m.mu.Unlock()
		}
	}
	return
}

func (m *Manager) CleanupExpiredSessions() (result []*Room) {
	rooms := m.Rooms()
	result = make([]*Room, 0)
	for _, room := range rooms {
		expired := room.expiredPlayers()
		if len(expired) == 0 {
			continue
		}
		result = append(result, room)
		for _, player := range expired {
			if room.leave(player) {
				m.mu.Lock()
				if m.rooms[room.code] == room {
					delete(m.rooms, room.code)
				}
				m.mu.Unlock()
				break
			}
		}
	}
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

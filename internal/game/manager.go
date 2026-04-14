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

func (m *Manager) CreateRoom(player *Player, defaultDeckIDs []string) (result1 *Room, errResult error) {
	if player == nil {
		result1, errResult = nil, ErrAlreadyInRoom
		return
	}
	if player.Room != nil {
		result1, errResult = nil, ErrAlreadyInRoom
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	code, err := m.newRoomCodeLocked()
	if err != nil {
		result1, errResult = nil, err
		return
	}
	roomRand, err := newCryptoRand()
	if err != nil {
		result1, errResult = nil, err
		return
	}
	room := &Room{
		manager:         m,
		code:            code,
		catalog:         m.catalog,
		rand:            roomRand,
		minPlayers:      m.opts.MinPlayers,
		debug:           m.opts.Debug,
		reviewDelay:     ReviewDelay,
		targetScore:     ScoreGoal,
		state:           StateLobby,
		czarIndex:       -1,
		selectedDeckIDs: normalizeDeckIDs(m.catalog, defaultDeckIDs),
	}
	room.seatLocked(player)
	room.host = player
	room.players = []*Player{player}
	m.rooms[code] = room
	result1, errResult = room, nil
	return
}

func (m *Manager) Room(code string) (result *Room) {
	m.mu.RLock()
	result = m.rooms[strings.ToUpper(strings.TrimSpace(code))]
	m.mu.RUnlock()
	return
}

func (m *Manager) GetRoom(code string) (result *Room) {
	result = m.Room(code)
	return
}

func (m *Manager) Rooms() (result []*Room) {
	m.mu.RLock()
	result = make([]*Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		result = append(result, room)
	}
	m.mu.RUnlock()
	slices.SortFunc(result, func(a, b *Room) (result int) { result = strings.Compare(a.code, b.code); return })
	return
}

func (m *Manager) PublicRooms() (result []*Room) {
	rooms := m.Rooms()
	result = rooms[:0]
	for _, room := range rooms {
		if !room.IsPrivate() {
			result = append(result, room)
		}
	}
	return
}

func (m *Manager) JoinRoom(code string, player *Player) (result1 *Room, errResult error) {
	if player == nil {
		result1, errResult = nil, ErrRoomNotFound
		return
	}
	room := m.Room(code)
	if room == nil {
		result1, errResult = nil, ErrRoomNotFound
		return
	}
	if player.Room == room {
		result1, errResult = room, nil
		return
	}
	if player.Room != nil {
		result1, errResult = nil, ErrAlreadyInRoom
		return
	}
	if err := room.join(player); err != nil {
		result1, errResult = nil, err
		return
	}
	result1, errResult = room, nil
	return
}

func (m *Manager) LeaveRoom(player *Player) (result1 *Room, result2 bool) {
	if player == nil || player.Room == nil {
		result1, result2 = nil, false
		return
	}
	room := player.Room
	destroy := room.leave(player)
	if destroy {
		m.mu.Lock()
		if m.rooms[room.code] == room {
			delete(m.rooms, room.code)
		}
		m.mu.Unlock()
	}
	result1, result2 = room, destroy
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
			destroy := room.leave(player)
			if destroy {
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

func (m *Manager) newRoomCodeLocked() (result1 string, errResult error) {
	for i := 0; i < 1024; i++ {
		code, err := randomCode()
		if err != nil {
			result1, errResult = "", err
			return
		}
		if _, exists := m.rooms[code]; !exists {
			result1, errResult = code, nil
			return
		}
	}
	result1, errResult = "", errors.New("could not allocate room code")
	return
}

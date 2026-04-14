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

func (m *Manager) CreateRoom(player *Player, defaultDeckIDs []string) (*Room, error) {
	if player == nil {
		return nil, ErrAlreadyInRoom
	}
	if player.Room != nil {
		return nil, ErrAlreadyInRoom
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	code, err := m.newRoomCodeLocked()
	if err != nil {
		return nil, err
	}
	roomRand, err := newCryptoRand()
	if err != nil {
		return nil, err
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
	return room, nil
}

func (m *Manager) Room(code string) *Room {
	m.mu.RLock()
	room := m.rooms[strings.ToUpper(strings.TrimSpace(code))]
	m.mu.RUnlock()
	return room
}

func (m *Manager) GetRoom(code string) *Room {
	return m.Room(code)
}

func (m *Manager) Rooms() []*Room {
	m.mu.RLock()
	rooms := make([]*Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		rooms = append(rooms, room)
	}
	m.mu.RUnlock()
	slices.SortFunc(rooms, func(a, b *Room) int { return strings.Compare(a.code, b.code) })
	return rooms
}

func (m *Manager) PublicRooms() []*Room {
	rooms := m.Rooms()
	public := rooms[:0]
	for _, room := range rooms {
		if !room.IsPrivate() {
			public = append(public, room)
		}
	}
	return public
}

func (m *Manager) JoinRoom(code string, player *Player) (*Room, error) {
	if player == nil {
		return nil, ErrRoomNotFound
	}
	room := m.Room(code)
	if room == nil {
		return nil, ErrRoomNotFound
	}
	if player.Room == room {
		return room, nil
	}
	if player.Room != nil {
		return nil, ErrAlreadyInRoom
	}
	if err := room.join(player); err != nil {
		return nil, err
	}
	return room, nil
}

func (m *Manager) LeaveRoom(player *Player) (*Room, bool) {
	if player == nil || player.Room == nil {
		return nil, false
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
	return room, destroy
}

func (m *Manager) CleanupExpiredSessions() []*Room {
	rooms := m.Rooms()
	affected := make([]*Room, 0)
	for _, room := range rooms {
		expired := room.expiredPlayers()
		if len(expired) == 0 {
			continue
		}
		affected = append(affected, room)
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
	return affected
}

func (m *Manager) newRoomCodeLocked() (string, error) {
	for i := 0; i < 1024; i++ {
		code, err := randomCode()
		if err != nil {
			return "", err
		}
		if _, exists := m.rooms[code]; !exists {
			return code, nil
		}
	}
	return "", errors.New("could not allocate room code")
}

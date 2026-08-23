package ui

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"net/http"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/game"
)

func (a *App) debugEnabled() (result bool) {
	result = a != nil && a.Jaws != nil && a.Jaws.Debug && a.Jaws.Logger != nil
	return
}

func (a *App) debugInfo(msg string, args ...any) {
	if a.debugEnabled() {
		a.Jaws.Logger.Info(msg, args...)
	}
}

func debugSessionLabel(sess *jaws.Session) (result string) {
	if sess != nil {
		var raw [8]byte
		binary.LittleEndian.PutUint64(raw[:], sess.ID())
		sum := sha256.Sum256(raw[:])
		result = hex.EncodeToString(sum[:6])
	}
	return
}

func debugPlayerSessionLabel(player *game.Player) (result string) {
	if player != nil {
		result = debugSessionLabel(player.Session)
	}
	return
}

func debugRoomCode(player *game.Player) (result string) {
	if player != nil {
		if room := player.Room(); room != nil {
			result = room.Code()
		}
	}
	return
}

func debugRequestPath(r *http.Request) (result string) {
	if r != nil && r.URL != nil {
		result = r.URL.Path
	}
	return
}

func (a *App) debugConnectionStarted(rq *jaws.Request, player *game.Player) {
	if !a.debugEnabled() || rq == nil {
		return
	}

	sess := rq.Session()
	ctx := rq.Context()
	session := debugSessionLabel(sess)
	path := debugRequestPath(rq.Initial())

	a.debugMu.Lock()
	if a.debugConnections == nil {
		a.debugConnections = make(map[*jaws.Session]int)
	}
	a.debugConnectionID++
	connection := a.debugConnectionID
	a.debugEventID++
	event := a.debugEventID
	a.debugConnections[sess]++
	connections := a.debugConnections[sess]
	activeSessions := len(a.debugConnections)
	a.debugMu.Unlock()
	jawsActiveSessions := a.Jaws.ActiveSessionCount()
	registeredSessions := a.Jaws.SessionCount()

	if connections == 1 {
		a.debugInfo(
			"JaWS session activity started",
			"event", event,
			"session", session,
			"tracked_active_sessions", activeSessions,
			"jaws_active_sessions", jawsActiveSessions,
			"registered_sessions", registeredSessions,
		)
	}
	a.debugInfo(
		"JaWS connection started",
		"event", event,
		"connection", connection,
		"session", session,
		"session_connections", connections,
		"tracked_active_sessions", activeSessions,
		"jaws_active_sessions", jawsActiveSessions,
		"registered_sessions", registeredSessions,
		"path", path,
		"room", debugRoomCode(player),
		"player", a.playerNickname(player),
	)

	context.AfterFunc(ctx, func() {
		a.debugConnectionStopped(connection, sess, session, path, player)
	})
}

func (a *App) debugConnectionStopped(connection uint64, sess *jaws.Session, session, path string, player *game.Player) {
	a.debugMu.Lock()
	a.debugEventID++
	event := a.debugEventID
	connections := a.debugConnections[sess]
	if connections > 1 {
		connections--
		a.debugConnections[sess] = connections
	} else {
		connections = 0
		delete(a.debugConnections, sess)
	}
	activeSessions := len(a.debugConnections)
	a.debugMu.Unlock()

	a.debugInfo(
		"JaWS connection stopped",
		"event", event,
		"connection", connection,
		"session", session,
		"session_connections", connections,
		"tracked_active_sessions", activeSessions,
		"path", path,
		"room", debugRoomCode(player),
		"player", a.playerNickname(player),
	)
	if connections == 0 {
		a.debugInfo(
			"JaWS session activity stopped",
			"event", event,
			"session", session,
			"tracked_active_sessions", activeSessions,
			"registered_sessions", a.Jaws.SessionCount(),
		)
	}
}

package ui

import (
	"context"
	"sync"
	"testing"

	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/game"
)

type debugLogRecord struct {
	level string
	msg   string
	args  []any
}

func (r debugLogRecord) value(key string) (result any) {
	for i := 0; i+1 < len(r.args); i += 2 {
		if r.args[i] == key {
			result = r.args[i+1]
			return
		}
	}
	return
}

type debugLogRecorder struct {
	records chan debugLogRecord
	mu      sync.Mutex
	history []debugLogRecord
}

func newDebugLogRecorder() (result *debugLogRecorder) {
	result = &debugLogRecorder{records: make(chan debugLogRecord, 256)}
	return
}

func (l *debugLogRecorder) record(level, msg string, args ...any) {
	record := debugLogRecord{level: level, msg: msg, args: append([]any(nil), args...)}
	l.mu.Lock()
	l.history = append(l.history, record)
	l.mu.Unlock()
	l.records <- record
}

func (l *debugLogRecorder) count(msg string, match func(debugLogRecord) bool) (result int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, record := range l.history {
		if record.msg == msg && (match == nil || match(record)) {
			result++
		}
	}
	return
}

func (l *debugLogRecorder) Info(msg string, args ...any) {
	l.record("info", msg, args...)
}

func (l *debugLogRecorder) Warn(msg string, args ...any) {
	l.record("warn", msg, args...)
}

func (l *debugLogRecorder) Error(msg string, args ...any) {
	l.record("error", msg, args...)
}

func waitDebugLog(t *testing.T, logger *debugLogRecorder, msg string, match func(debugLogRecord) bool) (result debugLogRecord) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer cancel()
	for {
		select {
		case record := <-logger.records:
			if record.msg == msg && (match == nil || match(record)) {
				result = record
				return
			}
		case <-ctx.Done():
			t.Fatalf("waiting for %q: %v", msg, context.Cause(ctx))
		}
	}
}

func newDebugTestApp(t *testing.T, logger jaws.Logger, enabled bool) (result *App) {
	t.Helper()
	jw, err := jaws.New()
	if err != nil {
		t.Fatal(err)
	}
	jw.Debug = enabled
	jw.Logger = logger
	t.Cleanup(jw.Close)
	go jw.Serve()
	catalog := testCatalog(t)
	result = New(jw, catalog, game.NewManager(catalog))
	return
}

func waitDebugRequestStopped(t *testing.T, requestContext context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer cancel()
	select {
	case <-requestContext.Done():
	case <-ctx.Done():
		t.Fatalf("waiting for Request to stop: %v", context.Cause(ctx))
	}
}

func TestDebugLogsPlayerRoomLifecycle(t *testing.T) {
	logger := newDebugLogRecorder()
	app := newDebugTestApp(t, logger, true)

	hostSession := newTestSession(t, app)
	host := app.player(hostSession, nil)
	app.Manager.SetNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatal(err)
	}

	guestSession := newTestSession(t, app)
	guest := app.player(guestSession, nil)
	app.Manager.SetNickname(guest, "Bob")
	if _, err = app.joinRoom(guest, room.Code()); err != nil {
		t.Fatal(err)
	}
	if left := app.leaveRoom(guest); left != room {
		t.Fatalf("leaveRoom() = %v, want %v", left, room)
	}

	hostSessionLabel := debugSessionLabel(hostSession)
	guestSessionLabel := debugSessionLabel(guestSession)
	if hostSessionLabel == "" || guestSessionLabel == "" || hostSessionLabel == guestSessionLabel {
		t.Fatalf("session labels = (%q, %q), want distinct non-empty labels", hostSessionLabel, guestSessionLabel)
	}
	waitDebugLog(t, logger, "player created", func(record debugLogRecord) bool {
		return record.value("session") == hostSessionLabel
	})
	waitDebugLog(t, logger, "player entered room", func(record debugLogRecord) bool {
		return record.value("reason") == "create" && record.value("player") == "Alice"
	})
	waitDebugLog(t, logger, "player created", func(record debugLogRecord) bool {
		return record.value("session") == guestSessionLabel
	})
	waitDebugLog(t, logger, "player entered room", func(record debugLogRecord) bool {
		return record.value("reason") == "join" && record.value("player") == "Bob" && record.value("players") == 2
	})
	waitDebugLog(t, logger, "player left room", func(record debugLogRecord) bool {
		return record.value("room") == room.Code() && record.value("player") == "Bob" && record.value("remaining_players") == 1
	})
}

func TestDebugDisabledDoesNotLogPlayerRoomLifecycle(t *testing.T) {
	logger := newDebugLogRecorder()
	app := newDebugTestApp(t, logger, false)
	sess := newTestSession(t, app)
	player := app.player(sess, nil)
	room, err := app.createRoom(player)
	if err != nil {
		t.Fatal(err)
	}
	app.leaveRoom(player)

	select {
	case record := <-logger.records:
		t.Fatalf("debug-disabled log = %#v", record)
	default:
	}
	if room == nil {
		t.Fatal("createRoom() returned nil")
	}
}

func TestDebugLogsExpiredPlayerDeparture(t *testing.T) {
	logger := newDebugLogRecorder()
	h := newConfiguredHarness(t, testCatalog(t), game.Options{}, func(jw *jaws.Jaws) {
		jw.Debug = true
		jw.Logger = logger
	})

	h.get(t, "/")
	sess := h.session(t)
	player := h.app.player(sess, nil)
	h.app.Manager.SetNickname(player, "Alice")
	room, err := h.app.createRoom(player)
	if err != nil {
		t.Fatal(err)
	}
	sess.Close()
	h.app.cleanupExpired()

	record := waitDebugLog(t, logger, "player left room", func(record debugLogRecord) bool {
		return record.value("reason") == "session_expired"
	})
	if got := record.value("session"); got != debugSessionLabel(sess) {
		t.Fatalf("session = %v, want %q", got, debugSessionLabel(sess))
	}
	if got := record.value("player"); got != "Alice" {
		t.Fatalf("player = %v, want Alice", got)
	}
	if got := record.value("room"); got != room.Code() {
		t.Fatalf("room = %v, want %q", got, room.Code())
	}
}

func TestDebugTracksSeveralConnectionsForOneSession(t *testing.T) {
	logger := newDebugLogRecorder()
	h := newConfiguredHarness(t, testCatalog(t), game.Options{}, func(jw *jaws.Jaws) {
		jw.Debug = true
		jw.Logger = logger
	})

	firstHTML := h.get(t, "/?secret=not-logged")
	sess := h.session(t)
	firstRequest := immediateModeRequestForHTML(t, sess, firstHTML)
	firstConn, firstCancel := h.connect(t, firstHTML)
	defer firstCancel()
	label := debugSessionLabel(sess)
	started := waitDebugLog(t, logger, "JaWS session activity started", func(record debugLogRecord) bool {
		return record.value("session") == label
	})
	if got := started.value("tracked_active_sessions"); got != 1 {
		t.Fatalf("tracked_active_sessions = %v, want 1", got)
	}
	firstStart := waitDebugLog(t, logger, "JaWS connection started", func(record debugLogRecord) bool {
		return record.value("session") == label
	})
	firstEvent, ok := firstStart.value("event").(uint64)
	if !ok || firstEvent == 0 {
		t.Fatalf("first event = %#v, want a positive uint64", firstStart.value("event"))
	}
	if got := started.value("event"); got != firstEvent {
		t.Fatalf("session-start event = %#v, want %d", got, firstEvent)
	}
	firstRequestContext := firstRequest.Context()
	if got := firstStart.value("path"); got != "/" {
		t.Fatalf("logged path = %v, want /", got)
	}

	secondHTML := h.get(t, "/")
	if h.session(t) != sess {
		t.Fatal("second connection did not reuse the first Session")
	}
	secondRequest := immediateModeRequestForHTML(t, sess, secondHTML)
	secondConn, secondCancel := h.connect(t, secondHTML)
	defer secondCancel()
	secondStart := waitDebugLog(t, logger, "JaWS connection started", func(record debugLogRecord) bool {
		return record.value("session") == label && record.value("session_connections") == 2
	})
	secondEvent, ok := secondStart.value("event").(uint64)
	if !ok || secondEvent <= firstEvent {
		t.Fatalf("second event = %#v, want greater than %d", secondStart.value("event"), firstEvent)
	}
	secondRequestContext := secondRequest.Context()
	if got := secondStart.value("tracked_active_sessions"); got != 1 {
		t.Fatalf("second tracked_active_sessions = %v, want 1", got)
	}
	if count := logger.count("JaWS session activity started", func(record debugLogRecord) bool {
		return record.value("session") == label
	}); count != 1 {
		t.Fatalf("session activity started logs = %d, want 1", count)
	}

	if err := secondConn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	waitDebugRequestStopped(t, secondRequestContext)
	secondStop := waitDebugLog(t, logger, "JaWS connection stopped", func(record debugLogRecord) bool {
		return record.value("session") == label && record.value("session_connections") == 1
	})
	secondStopEvent, ok := secondStop.value("event").(uint64)
	if !ok || secondStopEvent <= secondEvent {
		t.Fatalf("second-stop event = %#v, want greater than %d", secondStop.value("event"), secondEvent)
	}
	if got := secondStop.value("tracked_active_sessions"); got != 1 {
		t.Fatalf("tracked_active_sessions after one stop = %v, want 1", got)
	}
	if count := logger.count("JaWS session activity stopped", func(record debugLogRecord) bool {
		return record.value("session") == label
	}); count != 0 {
		t.Fatalf("premature session activity stopped logs = %d, want 0", count)
	}

	if err := firstConn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	waitDebugRequestStopped(t, firstRequestContext)
	firstStop := waitDebugLog(t, logger, "JaWS connection stopped", func(record debugLogRecord) bool {
		return record.value("session") == label && record.value("session_connections") == 0
	})
	stopped := waitDebugLog(t, logger, "JaWS session activity stopped", func(record debugLogRecord) bool {
		return record.value("session") == label
	})
	firstStopEvent, ok := firstStop.value("event").(uint64)
	if !ok || firstStopEvent <= secondStopEvent {
		t.Fatalf("first-stop event = %#v, want greater than %d", firstStop.value("event"), secondStopEvent)
	}
	if got := stopped.value("event"); got != firstStopEvent {
		t.Fatalf("session-stop event = %#v, want %d", got, firstStopEvent)
	}
	if got := stopped.value("tracked_active_sessions"); got != 0 {
		t.Fatalf("final tracked_active_sessions = %v, want 0", got)
	}
}

func TestDebugLogsTwoActiveSessions(t *testing.T) {
	logger := newDebugLogRecorder()
	h := newConfiguredHarness(t, testCatalog(t), game.Options{}, func(jw *jaws.Jaws) {
		jw.Debug = true
		jw.Logger = logger
	})

	h.get(t, "/")
	firstSession := h.session(t)
	firstPlayer := h.app.player(firstSession, nil)
	h.app.Manager.SetNickname(firstPlayer, "Alice")
	room, err := h.app.createRoom(firstPlayer)
	if err != nil {
		t.Fatal(err)
	}

	secondClient := h.newClient(t)
	h.getWithClient(t, secondClient, "/")
	secondSession := h.sessionForClient(t, secondClient)
	secondPlayer := h.app.player(secondSession, nil)
	h.app.Manager.SetNickname(secondPlayer, "Bob")
	if _, err = h.app.joinRoom(secondPlayer, room.Code()); err != nil {
		t.Fatal(err)
	}

	roomPath := "/room/" + room.Code()
	firstHTML := h.get(t, roomPath)
	firstConn, firstCancel := h.connect(t, firstHTML)
	defer firstCancel()
	defer func() {
		if err := firstConn.CloseNow(); err != nil {
			t.Errorf("closing first connection: %v", err)
		}
	}()
	firstLabel := debugSessionLabel(firstSession)
	firstStart := waitDebugLog(t, logger, "JaWS connection started", func(record debugLogRecord) bool {
		return record.value("session") == firstLabel
	})
	if got := firstStart.value("jaws_active_sessions"); got != 1 {
		t.Fatalf("first jaws_active_sessions = %v, want 1", got)
	}
	if got := firstStart.value("room"); got != room.Code() {
		t.Fatalf("first room = %v, want %q", got, room.Code())
	}
	if got := firstStart.value("player"); got != "Alice" {
		t.Fatalf("first player = %v, want Alice", got)
	}

	secondHTML := h.getWithClient(t, secondClient, roomPath)
	secondConn, secondCancel := h.connectWithClient(t, secondClient, secondHTML)
	defer secondCancel()
	secondLabel := debugSessionLabel(secondSession)
	if secondLabel == firstLabel {
		t.Fatalf("session labels = %q, want distinct values", firstLabel)
	}
	secondStart := waitDebugLog(t, logger, "JaWS connection started", func(record debugLogRecord) bool {
		return record.value("session") == secondLabel
	})
	if got := secondStart.value("tracked_active_sessions"); got != 2 {
		t.Fatalf("second tracked active_sessions = %v, want 2", got)
	}
	if got := secondStart.value("jaws_active_sessions"); got != 2 {
		t.Fatalf("second jaws_active_sessions = %v, want 2", got)
	}
	if got := secondStart.value("room"); got != room.Code() {
		t.Fatalf("second room = %v, want %q", got, room.Code())
	}
	if got := secondStart.value("player"); got != "Bob" {
		t.Fatalf("second player = %v, want Bob", got)
	}

	secondRequest := immediateModeRequestForHTML(t, secondSession, secondHTML)
	secondRequestContext := secondRequest.Context()
	if err := secondConn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	waitDebugRequestStopped(t, secondRequestContext)
	secondStop := waitDebugLog(t, logger, "JaWS connection stopped", func(record debugLogRecord) bool {
		return record.value("session") == secondLabel
	})
	if got := secondStop.value("tracked_active_sessions"); got != 1 {
		t.Fatalf("active_sessions after second stop = %v, want 1", got)
	}
}

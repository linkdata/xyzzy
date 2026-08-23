package ui

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/jaws/lib/jid"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/jaws/lib/what"
	"github.com/linkdata/jaws/lib/wire"
	"github.com/linkdata/xyzzy/internal/game"
)

const immediateModeTestTimeout = 5 * time.Second

var (
	immediateModeAttrRE                 = regexp.MustCompile(`(?i)\b([a-z][a-z0-9:_-]*)\s*=\s*"([^"]*)"`)
	immediateModeFirstTagRE             = regexp.MustCompile(`(?is)^\s*<[a-z][a-z0-9:_-]*\b[^>]*>`)
	immediateModeInputRE                = regexp.MustCompile(`(?is)<input\b[^>]*>`)
	immediateModeNicknameModalTriggerRE = regexp.MustCompile(`(?is)<button\b[^>]*\bdata-bs-toggle="modal"[^>]*>.*?</button>`)
	immediateModeSpanRE                 = regexp.MustCompile(`(?is)<span\b[^>]*>`)
)

type immediateModeChildChanges struct {
	removed  []jid.Jid
	appended []wire.WsMsg
}

func (changes *immediateModeChildChanges) observe(parent jid.Jid, msg wire.WsMsg) {
	if msg.Jid == parent {
		switch msg.What {
		case what.Remove:
			changes.removed = append(changes.removed, jid.ParseString(strings.TrimSpace(msg.Data)))
		case what.Append:
			changes.appended = append(changes.appended, msg)
		}
	}
}

func (changes immediateModeChildChanges) removedChild(child jid.Jid) (result bool) {
	for _, removed := range changes.removed {
		if removed == child {
			result = true
			return
		}
	}
	return
}

func (changes immediateModeChildChanges) appendedContaining(text string) (result bool) {
	for _, msg := range changes.appended {
		if strings.Contains(msg.Data, text) {
			result = true
			return
		}
	}
	return
}

type immediateModeWireReader struct {
	messages <-chan wire.WsMsg
	errors   <-chan error
}

func newImmediateModeWireReader(t *testing.T, conn *websocket.Conn) (result immediateModeWireReader) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	messages := make(chan wire.WsMsg, 256)
	errors := make(chan error, 1)
	t.Cleanup(cancel)
	go func() {
		defer close(messages)
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				select {
				case errors <- err:
				default:
				}
				return
			}
			for _, record := range bytes.SplitAfter(data, []byte{'\n'}) {
				if len(record) == 0 || record[len(record)-1] != '\n' {
					continue
				}
				if msg, ok := wire.Parse(record); ok {
					select {
					case messages <- msg:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	result = immediateModeWireReader{messages: messages, errors: errors}
	return
}

func (r immediateModeWireReader) readUntil(ctx context.Context, visit func(wire.WsMsg) bool) (err error) {
	for {
		select {
		case msg, ok := <-r.messages:
			if !ok {
				select {
				case err = <-r.errors:
				default:
					err = io.EOF
				}
				return
			}
			if visit(msg) {
				return
			}
		case err = <-r.errors:
			return
		case <-ctx.Done():
			err = context.Cause(ctx)
			return
		}
	}
}

func immediateModeRequestForHTML(t *testing.T, sess *jaws.Session, pageHTML string) (result *jaws.Request) {
	t.Helper()
	match := jawsKeyRe.FindStringSubmatch(pageHTML)
	if len(match) != 2 {
		t.Fatalf("page has no jawsKey: %s", pageHTML)
	}
	for _, rq := range sess.Requests() {
		if rq.JawsKeyString() == match[1] {
			result = rq
			return
		}
	}
	t.Fatalf("Session has no Request for jawsKey %q", match[1])
	return
}

func syncImmediateModeRequest(t *testing.T, conn *websocket.Conn, rq *jaws.Request, reader immediateModeWireReader, label string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	drainImmediateModeAlert(t, rq, reader, label, nil)
}

func drainImmediateModeAlert(t *testing.T, rq *jaws.Request, reader immediateModeWireReader, label string, visit func(wire.WsMsg)) {
	t.Helper()

	marker := "immediate-mode-barrier:" + t.Name() + ":" + label
	rq.Alert("info", marker)
	ctx, cancel := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer cancel()
	if err := reader.readUntil(ctx, func(msg wire.WsMsg) bool {
		if visit != nil {
			visit(msg)
		}
		return msg.What == what.Alert && strings.Contains(msg.Data, marker)
	}); err != nil {
		t.Fatalf("waiting for %s barrier: %v", label, err)
	}
}

func immediateModeAttrs(startTag string) (result map[string]string) {
	result = make(map[string]string)
	for _, match := range immediateModeAttrRE.FindAllStringSubmatch(startTag, -1) {
		result[strings.ToLower(match[1])] = match[2]
	}
	return
}

func immediateModeHasClasses(attrs map[string]string, classes ...string) (result bool) {
	set := make(map[string]bool)
	for _, class := range strings.Fields(attrs["class"]) {
		set[class] = true
	}
	result = true
	for _, class := range classes {
		if !set[class] {
			result = false
			return
		}
	}
	return
}

func immediateModeElementJID(t *testing.T, markup string, tags *regexp.Regexp, matches func(map[string]string) bool) (result jid.Jid) {
	t.Helper()
	for _, startTag := range tags.FindAllString(markup, -1) {
		attrs := immediateModeAttrs(startTag)
		if matches(attrs) {
			result = jid.ParseString(attrs["id"])
			if result <= 0 {
				t.Fatalf("managed start tag has invalid id: %s", startTag)
			}
			return
		}
	}
	t.Fatalf("managed element not found in markup: %s", markup)
	return
}

func immediateModeNicknameModalSpanJID(t *testing.T, markup string) (result jid.Jid) {
	t.Helper()
	triggers := immediateModeNicknameModalTriggerRE.FindAllString(markup, -1)
	if len(triggers) != 1 {
		t.Fatalf("nickname modal triggers = %d, want one: %s", len(triggers), markup)
	}
	trigger := triggers[0]
	buttonAttrs := immediateModeAttrs(immediateModeFirstTagRE.FindString(trigger))
	if buttonAttrs["data-bs-target"] != "#nicknameModal" {
		t.Fatalf("nickname modal target = %q, want #nicknameModal: %s", buttonAttrs["data-bs-target"], trigger)
	}
	if id := jid.ParseString(buttonAttrs["id"]); id > 0 {
		t.Fatalf("nickname modal trigger is managed as %v: %s", id, trigger)
	}
	spans := immediateModeSpanRE.FindAllString(trigger, -1)
	if len(spans) != 1 {
		t.Fatalf("nickname modal trigger spans = %d, want one: %s", len(spans), trigger)
	}
	spanAttrs := immediateModeAttrs(spans[0])
	result = jid.ParseString(spanAttrs["id"])
	if result <= 0 {
		t.Fatalf("nickname modal label is not managed: %s", trigger)
	}
	if !immediateModeHasClasses(spanAttrs, "pe-none") {
		t.Fatalf("nickname modal label is not pointer-transparent: %s", trigger)
	}
	return
}

func immediateModePrivateCheckboxJID(t *testing.T, markup string) (result jid.Jid) {
	t.Helper()
	result = immediateModeElementJID(t, markup, immediateModeInputRE, func(attrs map[string]string) bool {
		return strings.EqualFold(attrs["type"], "checkbox") && immediateModeHasClasses(attrs, "private-toggle-checkbox")
	})
	return
}

func immediateModeHTMLJIDs(markup string) (result map[jid.Jid]bool) {
	result = make(map[jid.Jid]bool)
	for _, match := range immediateModeAttrRE.FindAllStringSubmatch(markup, -1) {
		if strings.EqualFold(match[1], "id") {
			if id := jid.ParseString(match[2]); id > 0 {
				result[id] = true
			}
		}
	}
	return
}

func immediateModeRootJID(t *testing.T, markup string) (result jid.Jid) {
	t.Helper()
	startTag := immediateModeFirstTagRE.FindString(markup)
	if startTag == "" {
		t.Fatalf("markup has no root start tag: %s", markup)
	}
	result = jid.ParseString(immediateModeAttrs(startTag)["id"])
	if result <= 0 {
		t.Fatalf("root start tag has invalid id: %s", startTag)
	}
	return
}

func immediateModeTemplateJID(t *testing.T, rq *jaws.Request, pageHTML, name string) (result jid.Jid) {
	t.Helper()
	for id := range immediateModeHTMLJIDs(pageHTML) {
		if elem := rq.GetElementByJid(id); elem != nil {
			if tmpl, ok := elem.UI().(jui.Template); ok && tmpl.Name == name {
				if result > 0 {
					t.Fatalf("page has multiple %q Template elements", name)
				}
				result = id
			}
		}
	}
	if result <= 0 {
		t.Fatalf("page has no %q Template element", name)
	}
	return
}

func immediateModeButtonJID(t *testing.T, rq *jaws.Request, pageHTML, label string) (result jid.Jid) {
	t.Helper()
	for id := range immediateModeHTMLJIDs(pageHTML) {
		if elem := rq.GetElementByJid(id); elem != nil {
			if button, ok := elem.UI().(*jui.Button); ok && string(button.HTMLGetter.JawsGetHTML(elem)) == label {
				if result > 0 {
					t.Fatalf("page has multiple %q Button elements", label)
				}
				result = id
			}
		}
	}
	if result <= 0 {
		t.Fatalf("page has no %q Button element", label)
	}
	return
}

func immediateModeRoomSectionJIDs(t *testing.T, rq *jaws.Request, player *game.Player, manager *game.Manager) (sidebar, main jid.Jid) {
	t.Helper()
	for _, elem := range rq.GetElements(player) {
		if _, ok := elem.UI().(jui.Container); ok {
			if elem.HasTag(manager) {
				if main > 0 {
					t.Fatal("page has multiple Manager-tagged room Containers")
				}
				main = elem.Jid()
			} else {
				if sidebar > 0 {
					t.Fatal("page has multiple Player-only room Containers")
				}
				sidebar = elem.Jid()
			}
		}
	}
	if sidebar <= 0 || main <= 0 || sidebar == main {
		t.Fatalf("room section Jids = sidebar %v, main %v", sidebar, main)
	}
	return
}

func immediateModeFullRoom(t *testing.T, h *liveHarness) (room *game.Room, players []*game.Player) {
	t.Helper()
	_, host := livePlayer(t, h, "Alice")
	var err error
	room, err = h.app.Manager.CreateRoom(host, h.app.Catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	players = append(players, host)
	for i := 1; i < game.MaxPlayers; i++ {
		_, player := liveJoinedPlayer(t, h, fmt.Sprintf("Player%d", i))
		if _, err = h.app.Manager.JoinRoom(room.Code(), player); err != nil {
			t.Fatalf("JoinRoom(Player%d) error = %v", i, err)
		}
		players = append(players, player)
	}
	if got := room.PlayerCount(); got != game.MaxPlayers {
		t.Fatalf("PlayerCount() = %d, want %d", got, game.MaxPlayers)
	}
	return
}

func immediateModeContainerMutation(msg wire.WsMsg) (result bool) {
	switch msg.What {
	case what.Remove, what.Append, what.Order:
		result = true
	}
	return
}

func TestRoomConnectClaimsSeatOpenedAfterGET(t *testing.T) {
	h := newLiveHarness(t)
	room, players := immediateModeFullRoom(t, h)
	leaver := players[len(players)-1]

	viewerClient := h.newClient(t)
	fullHTML := h.getWithClient(t, viewerClient, "/room/"+room.Code())
	viewerSession := h.sessionForClient(t, viewerClient)
	viewer := h.app.player(viewerSession, nil)
	if viewer.Room() != nil || room.HasPlayer(viewer) {
		t.Fatal("full-room GET seated its viewer")
	}
	if !strings.Contains(fullHTML, "Not seated at this table") {
		t.Fatalf("full-room page did not render the unseated panel: %s", fullHTML)
	}
	rq := immediateModeRequestForHTML(t, viewerSession, fullHTML)
	sidebarJID, mainJID := immediateModeRoomSectionJIDs(t, rq, viewer, h.app.Manager)
	singleJID := immediateModeTemplateJID(t, rq, fullHTML, "room_single_panel.html")

	if leftRoom, empty := h.app.Manager.LeaveRoom(leaver); leftRoom != room || empty {
		t.Fatalf("LeaveRoom() = (%v, %v), want (%v, false)", leftRoom, empty, room)
	}
	conn, cancel := h.connectWithClient(t, viewerClient, fullHTML)
	defer cancel()
	reader := newImmediateModeWireReader(t, conn)
	var sidebarChanges, mainChanges immediateModeChildChanges
	observe := func(msg wire.WsMsg) {
		sidebarChanges.observe(sidebarJID, msg)
		mainChanges.observe(mainJID, msg)
	}
	ctx, done := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	if err := reader.readUntil(ctx, func(msg wire.WsMsg) bool {
		observe(msg)
		return sidebarChanges.appendedContaining(room.Code()) &&
			mainChanges.removedChild(singleJID) && mainChanges.appendedContaining("Card Packs")
	}); err != nil {
		done()
		t.Fatalf("waiting for connect-time room reconciliation: %v", err)
	}
	done()
	drainImmediateModeAlert(t, rq, reader, "connected-after-seat-opened", observe)

	if viewer.Room() != room || !room.HasPlayer(viewer) {
		t.Fatal("JaWS connection did not join the viewer")
	}
	if got := room.PlayerCount(); got != game.MaxPlayers {
		t.Fatalf("PlayerCount() after connect = %d, want %d", got, game.MaxPlayers)
	}
	if len(sidebarChanges.removed) != 0 || len(sidebarChanges.appended) != 1 {
		t.Fatalf("sidebar child changes = removed %v, appended %d; want 0/1", sidebarChanges.removed, len(sidebarChanges.appended))
	}
	if len(mainChanges.removed) != 1 || mainChanges.removed[0] != singleJID || len(mainChanges.appended) != 1 {
		t.Fatalf("main child changes = removed %v, appended %d; want [%v]/1", mainChanges.removed, len(mainChanges.appended), singleJID)
	}
}

func TestRoomMembershipChangesReconcileSectionChildren(t *testing.T) {
	h := newLiveHarness(t)
	room, players := immediateModeFullRoom(t, h)

	viewerClient := h.newClient(t)
	pageHTML := h.getWithClient(t, viewerClient, "/room/"+room.Code())
	viewerSession := h.sessionForClient(t, viewerClient)
	viewer := h.app.player(viewerSession, nil)
	rq := immediateModeRequestForHTML(t, viewerSession, pageHTML)
	sidebarJID, mainJID := immediateModeRoomSectionJIDs(t, rq, viewer, h.app.Manager)
	singleJID := immediateModeTemplateJID(t, rq, pageHTML, "room_single_panel.html")

	conn, cancel := h.connectWithClient(t, viewerClient, pageHTML)
	defer cancel()
	reader := newImmediateModeWireReader(t, conn)
	syncImmediateModeRequest(t, conn, rq, reader, "membership-connected")
	if viewer.Room() != nil || room.HasPlayer(viewer) {
		t.Fatal("full-room connection seated its viewer")
	}
	if got := room.PlayerCount(); got != game.MaxPlayers {
		t.Fatalf("PlayerCount() after full-room connection = %d, want %d", got, game.MaxPlayers)
	}

	leaver := players[len(players)-1]
	if leftRoom, empty := h.app.Manager.LeaveRoom(leaver); leftRoom != room || empty {
		t.Fatalf("LeaveRoom() = (%v, %v), want (%v, false)", leftRoom, empty, room)
	}
	if joinedRoom, err := h.app.joinRoom(viewer, room.Code()); err != nil || joinedRoom != room {
		t.Fatalf("joinRoom() = (%v, %v), want (%v, nil)", joinedRoom, err, room)
	}

	var joinedSidebar, joinedMain immediateModeChildChanges
	observeJoin := func(msg wire.WsMsg) {
		joinedSidebar.observe(sidebarJID, msg)
		joinedMain.observe(mainJID, msg)
	}
	joinCtx, joinDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer joinDone()
	if err := reader.readUntil(joinCtx, func(msg wire.WsMsg) bool {
		observeJoin(msg)
		return joinedSidebar.appendedContaining(room.Code()) &&
			joinedMain.removedChild(singleJID) && joinedMain.appendedContaining("Card Packs")
	}); err != nil {
		t.Fatalf("waiting for joined room-section reconciliation: %v", err)
	}
	drainImmediateModeAlert(t, rq, reader, "membership-joined", observeJoin)

	if len(joinedSidebar.removed) != 0 || len(joinedSidebar.appended) != 1 {
		t.Fatalf("joined sidebar child changes = removed %v, appended %d; want 0/1", joinedSidebar.removed, len(joinedSidebar.appended))
	}
	if len(joinedMain.removed) != 1 || joinedMain.removed[0] != singleJID || len(joinedMain.appended) != 1 {
		t.Fatalf("joined main child changes = removed %v, appended %d; want [%v]/1", joinedMain.removed, len(joinedMain.appended), singleJID)
	}
	summaryJID := immediateModeRootJID(t, joinedSidebar.appended[0].Data)
	gameJID := immediateModeRootJID(t, joinedMain.appended[0].Data)
	if summaryJID == gameJID || summaryJID == singleJID || gameJID == singleJID {
		t.Fatalf("single/summary/game child Jids = %v/%v/%v, want distinct identities", singleJID, summaryJID, gameJID)
	}

	if leftRoom := h.app.leaveRoom(viewer); leftRoom != room {
		t.Fatalf("leaveRoom() = %v, want %v", leftRoom, room)
	}
	var leftSidebar, leftMain immediateModeChildChanges
	observeLeave := func(msg wire.WsMsg) {
		leftSidebar.observe(sidebarJID, msg)
		leftMain.observe(mainJID, msg)
	}
	leaveCtx, leaveDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer leaveDone()
	if err := reader.readUntil(leaveCtx, func(msg wire.WsMsg) bool {
		observeLeave(msg)
		return leftSidebar.removedChild(summaryJID) &&
			leftMain.removedChild(gameJID) && leftMain.appendedContaining("Not seated at this table")
	}); err != nil {
		t.Fatalf("waiting for left room-section reconciliation: %v", err)
	}
	drainImmediateModeAlert(t, rq, reader, "membership-left", observeLeave)

	if len(leftSidebar.removed) != 1 || leftSidebar.removed[0] != summaryJID || len(leftSidebar.appended) != 0 {
		t.Fatalf("left sidebar child changes = removed %v, appended %d; want [%v]/0", leftSidebar.removed, len(leftSidebar.appended), summaryJID)
	}
	if len(leftMain.removed) != 1 || leftMain.removed[0] != gameJID || len(leftMain.appended) != 1 {
		t.Fatalf("left main child changes = removed %v, appended %d; want [%v]/1", leftMain.removed, len(leftMain.appended), gameJID)
	}
	newSingleJID := immediateModeRootJID(t, leftMain.appended[0].Data)
	if newSingleJID == singleJID || newSingleJID == summaryJID || newSingleJID == gameJID {
		t.Fatalf("recreated single-panel child Jid = %v, want a new identity", newSingleJID)
	}
}

func TestCleanupRefreshesRemainingPlayerRoomSummary(t *testing.T) {
	h := newLiveHarness(t)
	hostSession, host := livePlayer(t, h, "Alice")
	room, err := h.app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestClient := h.newClient(t)
	h.getWithClient(t, guestClient, "/")
	guestSession := h.sessionForClient(t, guestClient)
	guest := h.app.player(guestSession, nil)
	h.app.Manager.SetNickname(guest, "Bob")
	if _, err = h.app.joinRoom(guest, room.Code()); err != nil {
		t.Fatalf("joinRoom() error = %v", err)
	}

	pageHTML := h.getWithClient(t, guestClient, "/room/"+room.Code())
	rq := immediateModeRequestForHTML(t, guestSession, pageHTML)
	sidebarJID, mainJID := immediateModeRoomSectionJIDs(t, rq, guest, h.app.Manager)
	summaryJID := immediateModeTemplateJID(t, rq, pageHTML, "room_summary_panel.html")
	conn, cancel := h.connectWithClient(t, guestClient, pageHTML)
	defer cancel()
	reader := newImmediateModeWireReader(t, conn)
	syncImmediateModeRequest(t, conn, rq, reader, "cleanup-connected")

	hostSession.Close()
	h.app.cleanupExpired()
	var summary wire.WsMsg
	var sidebarChanges, mainChanges immediateModeChildChanges
	observe := func(msg wire.WsMsg) {
		sidebarChanges.observe(sidebarJID, msg)
		mainChanges.observe(mainJID, msg)
	}
	ctx, done := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer done()
	if err = reader.readUntil(ctx, func(msg wire.WsMsg) bool {
		observe(msg)
		if msg.Jid == summaryJID && msg.What == what.Inner {
			summary = msg
			return true
		}
		return false
	}); err != nil {
		t.Fatalf("waiting for room summary cleanup update: %v", err)
	}
	drainImmediateModeAlert(t, rq, reader, "cleanup-refreshed", observe)

	if strings.Contains(summary.Data, "Alice") || !strings.Contains(summary.Data, "Bob") {
		t.Fatalf("room summary after cleanup = %q, want Bob without Alice", summary.Data)
	}
	if len(sidebarChanges.removed) != 0 || len(sidebarChanges.appended) != 0 ||
		len(mainChanges.removed) != 0 || len(mainChanges.appended) != 0 {
		t.Fatalf("cleanup changed room-section children: sidebar=%+v main=%+v", sidebarChanges, mainChanges)
	}
}

func TestCreateRoomDirtiesPlayerMembershipAcrossLiveRequests(t *testing.T) {
	app, mux := testApp(t)
	player := &game.Player{Nickname: "Alice", NicknameInput: "Alice"}
	roomCode := bind.StringGetterFunc(func(*jaws.Element) (result string) {
		result = "Not seated"
		if room := player.Room(); room != nil {
			result = room.Code()
		}
		return
	}, player)

	const templateName = "player_room_membership_test.html"
	templates, err := template.New(templateName).Parse(`<!doctype html>
<html><head>{{$.HeadHTML}}</head><body>{{$.Span .Dot}}{{$.TailHTML}}</body></html>`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err = app.Jaws.AddTemplateLookuper(templates); err != nil {
		t.Fatalf("AddTemplateLookuper() error = %v", err)
	}
	mux.Handle("GET /player-room-membership-test", jui.Handler(app.Jaws, templateName, roomCode))

	server := httptest.NewServer(app.Middleware(mux))
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	h := &liveHarness{app: app, server: server, base: base}
	h.client = h.newClient(t)
	otherClient := h.newClient(t)
	firstHTML := h.get(t, "/player-room-membership-test")
	secondHTML := h.getWithClient(t, otherClient, "/player-room-membership-test")
	spanJID := func(markup string) jid.Jid {
		return immediateModeElementJID(t, markup, immediateModeSpanRE, func(map[string]string) bool { return true })
	}
	firstJID := spanJID(firstHTML)
	secondJID := spanJID(secondHTML)

	firstConn, firstCancel := h.connect(t, firstHTML)
	defer firstCancel()
	secondConn, secondCancel := h.connectWithClient(t, otherClient, secondHTML)
	defer secondCancel()
	firstReader := newImmediateModeWireReader(t, firstConn)
	secondReader := newImmediateModeWireReader(t, secondConn)
	for i, conn := range []*websocket.Conn{firstConn, secondConn} {
		ctx, cancel := context.WithTimeout(t.Context(), immediateModeTestTimeout)
		if err = conn.Ping(ctx); err != nil {
			cancel()
			t.Fatalf("connection %d Ping() error = %v", i, err)
		}
		cancel()
	}

	room, err := app.createRoom(player)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}
	for i, peer := range []struct {
		jid    jid.Jid
		reader immediateModeWireReader
	}{
		{jid: firstJID, reader: firstReader},
		{jid: secondJID, reader: secondReader},
	} {
		ctx, cancel := context.WithTimeout(t.Context(), immediateModeTestTimeout)
		err = peer.reader.readUntil(ctx, func(msg wire.WsMsg) bool {
			return msg.Jid == peer.jid && msg.What == what.Inner && msg.Data == room.Code()
		})
		cancel()
		if err != nil {
			t.Fatalf("waiting for player membership on connection %d: %v", i, err)
		}
	}
}

func TestCreateRoomButtonCreatesRoom(t *testing.T) {
	h := newLiveHarness(t)

	pageHTML := h.get(t, "/")
	sess := h.session(t)
	player := h.app.player(sess, nil)
	rq := immediateModeRequestForHTML(t, sess, pageHTML)
	buttonJID := immediateModeButtonJID(t, rq, pageHTML, "Create Room")

	conn, cancel := h.connect(t, pageHTML)
	defer cancel()
	reader := newImmediateModeWireReader(t, conn)
	syncImmediateModeRequest(t, conn, rq, reader, "create-room-connected")

	ctx, done := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer done()
	click := wire.WsMsg{Jid: buttonJID, What: what.Click, Data: (jaws.Click{Name: "create"}).String()}
	if err := conn.Write(ctx, websocket.MessageText, []byte(click.Format())); err != nil {
		t.Fatalf("create-room Click write error = %v", err)
	}
	var redirect string
	if err := reader.readUntil(ctx, func(msg wire.WsMsg) bool {
		if msg.What == what.Redirect {
			redirect = msg.Data
			return true
		}
		return false
	}); err != nil {
		t.Fatalf("waiting for create-room redirect: %v", err)
	}
	room := player.Room()
	if room == nil {
		t.Fatal("Create Room click did not seat player")
	}
	if want := h.app.RoomURL(room.Code()); redirect != want {
		t.Fatalf("Create Room redirect = %q, want %q", redirect, want)
	}
}

func TestCreateRoomButtonWarnsWhenRateLimited(t *testing.T) {
	h := newLiveHarness(t)

	pageHTML := h.get(t, "/")
	sess := h.session(t)
	player := h.app.player(sess, nil)
	rq := immediateModeRequestForHTML(t, sess, pageHTML)
	buttonJID := immediateModeButtonJID(t, rq, pageHTML, "Create Room")
	ip := clientIP(rq.Initial())
	for attempt := range createRoomMinuteBurst {
		if !h.app.createRoomLimiter.Allow(ip) {
			t.Fatalf("limiter setup attempt %d rejected", attempt+1)
		}
	}

	conn, cancel := h.connect(t, pageHTML)
	defer cancel()
	reader := newImmediateModeWireReader(t, conn)
	syncImmediateModeRequest(t, conn, rq, reader, "limited-create-room-connected")

	ctx, done := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer done()
	click := wire.WsMsg{Jid: buttonJID, What: what.Click, Data: (jaws.Click{Name: "create"}).String()}
	if err := conn.Write(ctx, websocket.MessageText, []byte(click.Format())); err != nil {
		t.Fatalf("create-room Click write error = %v", err)
	}
	if err := reader.readUntil(ctx, func(msg wire.WsMsg) bool {
		return msg.What == what.Alert && strings.Contains(msg.Data, "Please wait before creating another room.")
	}); err != nil {
		t.Fatalf("waiting for rate-limit warning: %v", err)
	}
	if room := player.Room(); room != nil {
		t.Fatalf("rate-limited Create Room seated player in %s", room.Code())
	}
}

func TestRequestedRoomRemovalReplacesMainChild(t *testing.T) {
	h := newLiveHarness(t)
	room, players := immediateModeFullRoom(t, h)

	viewerClient := h.newClient(t)
	pageHTML := h.getWithClient(t, viewerClient, "/room/"+room.Code())
	viewerSession := h.sessionForClient(t, viewerClient)
	viewer := h.app.player(viewerSession, nil)
	rq := immediateModeRequestForHTML(t, viewerSession, pageHTML)
	sidebarJID, mainJID := immediateModeRoomSectionJIDs(t, rq, viewer, h.app.Manager)
	singleJID := immediateModeTemplateJID(t, rq, pageHTML, "room_single_panel.html")

	conn, cancel := h.connectWithClient(t, viewerClient, pageHTML)
	defer cancel()
	reader := newImmediateModeWireReader(t, conn)
	syncImmediateModeRequest(t, conn, rq, reader, "room-removal-connected")

	for i, player := range players {
		leftRoom, empty := h.app.Manager.LeaveRoom(player)
		wantEmpty := i == len(players)-1
		if leftRoom != room || empty != wantEmpty {
			t.Fatalf("LeaveRoom(player %d) = (%v, %v), want (%v, %v)", i, leftRoom, empty, room, wantEmpty)
		}
	}
	if got := h.app.Manager.Room(room.Code()); got != nil {
		t.Fatalf("Manager.Room(%q) = %v after removing every player", room.Code(), got)
	}
	h.app.Jaws.Dirty(h.app.Manager)

	var sidebarChanges, mainChanges immediateModeChildChanges
	observe := func(msg wire.WsMsg) {
		sidebarChanges.observe(sidebarJID, msg)
		mainChanges.observe(mainJID, msg)
	}
	ctx, done := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer done()
	if err := reader.readUntil(ctx, func(msg wire.WsMsg) bool {
		observe(msg)
		return mainChanges.removedChild(singleJID) && mainChanges.appendedContaining("Room not found")
	}); err != nil {
		t.Fatalf("waiting for removed-room reconciliation: %v", err)
	}
	drainImmediateModeAlert(t, rq, reader, "room-removed", observe)

	if len(sidebarChanges.removed) != 0 || len(sidebarChanges.appended) != 0 {
		t.Fatalf("removed room changed empty sidebar children: removed %v, appended %d", sidebarChanges.removed, len(sidebarChanges.appended))
	}
	if len(mainChanges.removed) != 1 || mainChanges.removed[0] != singleJID || len(mainChanges.appended) != 1 {
		t.Fatalf("removed-room main child changes = removed %v, appended %d; want [%v]/1", mainChanges.removed, len(mainChanges.appended), singleJID)
	}
	missingJID := immediateModeRootJID(t, mainChanges.appended[0].Data)
	if missingJID == singleJID {
		t.Fatalf("missing-room child reused removed Jid %v", singleJID)
	}
}

func TestRoomStateChangesReplaceOnlyTheStateTemplate(t *testing.T) {
	h := newHarnessWithCatalog(t, testPlayableCatalog(t), game.Options{MinPlayers: 2})

	h.get(t, "/")
	hostSession := h.session(t)
	host := h.app.player(hostSession, nil)
	room, err := h.app.Manager.CreateRoom(host, h.app.Catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	guestClient := h.newClient(t)
	h.getWithClient(t, guestClient, "/")
	guestSession := h.sessionForClient(t, guestClient)
	guest := h.app.player(guestSession, nil)
	if _, err = h.app.Manager.JoinRoom(room.Code(), guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}

	pageHTML := h.get(t, "/room/"+room.Code())
	rq := immediateModeRequestForHTML(t, hostSession, pageHTML)
	_, mainJID := immediateModeRoomSectionJIDs(t, rq, host, h.app.Manager)
	lobbyJID := immediateModeTemplateJID(t, rq, pageHTML, "room_game_lobby.html")
	conn, cancel := h.connect(t, pageHTML)
	defer cancel()
	reader := newImmediateModeWireReader(t, conn)
	syncImmediateModeRequest(t, conn, rq, reader, "state-connected")

	if err = room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	h.app.Jaws.Dirty(room)
	var changes immediateModeChildChanges
	transitionCtx, transitionDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer transitionDone()
	if err = reader.readUntil(transitionCtx, func(msg wire.WsMsg) bool {
		changes.observe(mainJID, msg)
		return changes.removedChild(lobbyJID) && changes.appendedContaining("Waiting for answers")
	}); err != nil {
		t.Fatalf("waiting for lobby-to-playing child transition: %v", err)
	}
	drainImmediateModeAlert(t, rq, reader, "state-transition", func(msg wire.WsMsg) {
		changes.observe(mainJID, msg)
	})
	if len(changes.removed) != 1 || len(changes.appended) != 1 {
		t.Fatalf("state child changes = removed %v, appended %d; want one each", changes.removed, len(changes.appended))
	}
	playingJID := immediateModeRootJID(t, changes.appended[0].Data)
	playingElem := rq.GetElementByJid(playingJID)
	if playingElem == nil {
		t.Fatalf("playing child %v is not registered", playingJID)
	}
	playingTemplate, ok := playingElem.UI().(jui.Template)
	if !ok || playingTemplate.Name != "room_game_playing.html" {
		t.Fatalf("playing child = %#v, want room_game_playing.html Template", playingElem)
	}

	var steadyMutations []wire.WsMsg
	h.app.Jaws.Dirty(room)
	steadyCtx, steadyDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer steadyDone()
	if err = reader.readUntil(steadyCtx, func(msg wire.WsMsg) bool {
		if immediateModeContainerMutation(msg) {
			steadyMutations = append(steadyMutations, msg)
		}
		return msg.Jid == playingJID && msg.What == what.Inner
	}); err != nil {
		t.Fatalf("waiting for same-state playing update: %v", err)
	}
	drainImmediateModeAlert(t, rq, reader, "state-steady", func(msg wire.WsMsg) {
		if immediateModeContainerMutation(msg) {
			steadyMutations = append(steadyMutations, msg)
		}
	})
	if len(steadyMutations) != 0 {
		t.Fatalf("same-state update emitted Container mutations: %#v", steadyMutations)
	}
}

func TestTargetScoreRangeInputUpdatesOnlyBoundControls(t *testing.T) {
	h := newPlayableLiveHarness(t)

	h.get(t, "/")
	sess := h.session(t)
	host := h.app.player(sess, nil)
	room, err := h.app.Manager.CreateRoom(host, h.app.Catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	pageHTML := h.get(t, "/room/"+room.Code())
	conn, cancel := h.connect(t, pageHTML)
	defer cancel()
	reader := newImmediateModeWireReader(t, conn)
	rq := immediateModeRequestForHTML(t, sess, pageHTML)
	syncImmediateModeRequest(t, conn, rq, reader, "connected")

	panelJID := immediateModeTemplateJID(t, rq, pageHTML, "room_game_lobby.html")
	rangeJID := immediateModeElementJID(t, pageHTML, immediateModeInputRE, func(attrs map[string]string) bool {
		return strings.EqualFold(attrs["type"], "range")
	})
	spanJID := immediateModeElementJID(t, pageHTML, immediateModeSpanRE, func(attrs map[string]string) bool {
		return immediateModeHasClasses(attrs, "badge", "bg-secondary", "text-nowrap")
	})
	if panelJID == rangeJID || panelJID == spanJID {
		t.Fatalf("panel/range/span Jids = %v/%v/%v, want distinct managed elements", panelJID, rangeJID, spanJID)
	}

	writeCtx, writeDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer writeDone()
	input := wire.WsMsg{Jid: rangeJID, What: what.Input, Data: "1"}
	if err := conn.Write(writeCtx, websocket.MessageText, []byte(input.Format())); err != nil {
		t.Fatalf("Range Input write error = %v", err)
	}

	want := strconv.Itoa(room.MinTargetScore())
	var sawRange, sawSpan, sawPanelInner bool
	observe := func(msg wire.WsMsg) {
		if msg.What == what.Inner && msg.Jid == panelJID {
			sawPanelInner = true
		}
		if msg.What == what.Value && msg.Jid == rangeJID && msg.Data == want {
			sawRange = true
		}
		if msg.What == what.Inner && msg.Jid == spanJID && msg.Data == want {
			sawSpan = true
		}
	}
	updateCtx, updateDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer updateDone()
	if err := reader.readUntil(updateCtx, func(msg wire.WsMsg) bool {
		observe(msg)
		return sawRange && sawSpan
	}); err != nil {
		t.Fatalf("waiting for Range and Span updates: %v", err)
	}
	drainImmediateModeAlert(t, rq, reader, "range-and-span-updated", observe)

	if got := room.TargetScore(); got != room.MinTargetScore() {
		t.Fatalf("TargetScore() = %d, want normalized %d", got, room.MinTargetScore())
	}
	if sawPanelInner {
		t.Fatalf("target-score input sent Inner to room panel %v", panelJID)
	}
}

func TestNicknameSaveReconcilesSiblingRequest(t *testing.T) {
	h := newLiveHarness(t)

	firstHTML := h.get(t, "/")
	sess := h.session(t)
	player := h.app.player(sess, nil)
	firstRQ := immediateModeRequestForHTML(t, sess, firstHTML)
	firstFields := firstRQ.GetElements(player.NicknameField())
	if len(firstFields) != 1 {
		t.Fatalf("first nickname fields = %#v, want one", firstFields)
	}
	firstFieldJID := firstFields[0].Jid()
	firstSaveJID := immediateModeButtonJID(t, firstRQ, firstHTML, "Save Nickname")

	secondHTML := h.get(t, "/")
	secondRQ := immediateModeRequestForHTML(t, sess, secondHTML)
	secondFields := secondRQ.GetElements(player.NicknameField())
	if len(secondFields) != 1 {
		t.Fatalf("second nickname fields = %#v, want one", secondFields)
	}
	secondFieldJID := secondFields[0].Jid()
	var secondDisplayJID jid.Jid
	for _, elem := range secondRQ.GetElements(player) {
		if _, ok := elem.UI().(*jui.Span); ok {
			if secondDisplayJID > 0 {
				t.Fatal("second page has multiple Player-tagged spans")
			}
			secondDisplayJID = elem.Jid()
		}
	}
	if secondDisplayJID <= 0 {
		t.Fatal("second page has no Player-tagged nickname span")
	}

	firstConn, firstCancel := h.connect(t, firstHTML)
	defer firstCancel()
	firstReader := newImmediateModeWireReader(t, firstConn)
	secondConn, secondCancel := h.connect(t, secondHTML)
	defer secondCancel()
	secondReader := newImmediateModeWireReader(t, secondConn)
	syncImmediateModeRequest(t, firstConn, firstRQ, firstReader, "nickname-first-connected")
	syncImmediateModeRequest(t, secondConn, secondRQ, secondReader, "nickname-second-connected")

	const raw = " A l i c e !!! "
	inputWriteCtx, inputWriteDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	input := wire.WsMsg{Jid: firstFieldJID, What: what.Input, Data: raw}
	if err := firstConn.Write(inputWriteCtx, websocket.MessageText, []byte(input.Format())); err != nil {
		inputWriteDone()
		t.Fatalf("nickname Input write error = %v", err)
	}
	inputWriteDone()
	peerRawCtx, peerRawDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer peerRawDone()
	if err := secondReader.readUntil(peerRawCtx, func(msg wire.WsMsg) bool {
		return msg.Jid == secondFieldJID && msg.What == what.Value && msg.Data == raw
	}); err != nil {
		t.Fatalf("waiting for sibling raw nickname Value: %v", err)
	}

	click := wire.WsMsg{Jid: firstSaveJID, What: what.Click, Data: (jaws.Click{Name: "save"}).String()}
	clickWriteCtx, clickWriteDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	if err := firstConn.Write(clickWriteCtx, websocket.MessageText, []byte(click.Format())); err != nil {
		clickWriteDone()
		t.Fatalf("save Click write error = %v", err)
	}
	clickWriteDone()
	peerSavedCtx, peerSavedDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer peerSavedDone()
	var sawField, sawDisplay bool
	if err := secondReader.readUntil(peerSavedCtx, func(msg wire.WsMsg) bool {
		if msg.Jid == secondFieldJID && msg.What == what.Value && msg.Data == "Alice" {
			sawField = true
		}
		if msg.Jid == secondDisplayJID && msg.What == what.Inner && msg.Data == "Alice" {
			sawDisplay = true
		}
		return sawField && sawDisplay
	}); err != nil {
		t.Fatalf("waiting for sibling normalized nickname display: field=%t display=%t: %v", sawField, sawDisplay, err)
	}

	if got := player.NicknameValue(); got != "Alice" {
		t.Fatalf("NicknameValue() = %q, want Alice", got)
	}
	if got := player.NicknameInputValue(); got != "Alice" {
		t.Fatalf("NicknameInputValue() = %q, want Alice", got)
	}
}

func TestNicknameModalTriggersLeaveClicksToBootstrap(t *testing.T) {
	h := newLiveHarness(t)

	lobbyHTML := h.get(t, "/")
	sess := h.session(t)
	player := h.app.player(sess, nil)
	room, err := h.app.createRoom(player)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}
	roomHTML := h.get(t, h.app.RoomURL(room.Code()))

	for name, pageHTML := range map[string]string{
		"lobby": lobbyHTML,
		"room":  roomHTML,
	} {
		t.Run(name, func(t *testing.T) {
			rq := immediateModeRequestForHTML(t, sess, pageHTML)
			spanJID := immediateModeNicknameModalSpanJID(t, pageHTML)
			span := rq.GetElementByJid(spanJID)
			if span == nil {
				t.Fatalf("nickname modal label %v is not registered", spanJID)
			}
			if _, ok := span.UI().(*jui.Span); !ok || !span.HasTag(player) {
				t.Fatalf("nickname modal label = %#v, want Player-tagged Span", span.UI())
			}
		})
	}
}

func TestReviewCountdownUpdatesJudgeAndNonJudgeControls(t *testing.T) {
	h := newHarnessWithCatalog(t, testPlayableCatalog(t), game.Options{MinPlayers: 2})

	hostClient := h.client
	h.getWithClient(t, hostClient, "/")
	hostSession := h.sessionForClient(t, hostClient)
	host := h.app.player(hostSession, nil)
	h.app.Manager.SetNickname(host, "Alice")
	room, err := h.app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestClient := h.newClient(t)
	h.getWithClient(t, guestClient, "/")
	guestSession := h.sessionForClient(t, guestClient)
	guest := h.app.player(guestSession, nil)
	h.app.Manager.SetNickname(guest, "Bob")
	if _, err = h.app.joinRoom(guest, room.Code()); err != nil {
		t.Fatalf("joinRoom() error = %v", err)
	}
	if err = room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !room.IsJudge(host) {
		t.Fatal("opening judge is not the host")
	}

	hand := room.HandFor(guest)
	if err = room.PlayCards(guest, hand[:room.NeedPick()]); err != nil {
		t.Fatalf("PlayCards() error = %v", err)
	}
	if err = room.Judge(host, room.Submissions()[0]); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	t.Cleanup(func() {
		if room.State() == game.StateReview {
			_ = room.ProceedReview(host)
		}
	})

	hostHTML := h.getWithClient(t, hostClient, "/room/"+room.Code())
	hostRQ := immediateModeRequestForHTML(t, hostSession, hostHTML)
	hostPanelJID := immediateModeTemplateJID(t, hostRQ, hostHTML, "room_game_review.html")

	guestHTML := h.getWithClient(t, guestClient, "/room/"+room.Code())
	guestRQ := immediateModeRequestForHTML(t, guestSession, guestHTML)
	guestPanelJID := immediateModeTemplateJID(t, guestRQ, guestHTML, "room_game_review.html")

	hostConn, hostCancel := h.connectWithClient(t, hostClient, hostHTML)
	defer hostCancel()
	hostReader := newImmediateModeWireReader(t, hostConn)
	guestConn, guestCancel := h.connectWithClient(t, guestClient, guestHTML)
	defer guestCancel()
	guestReader := newImmediateModeWireReader(t, guestConn)

	syncImmediateModeRequest(t, hostConn, hostRQ, hostReader, "review-host-connected")
	syncImmediateModeRequest(t, guestConn, guestRQ, guestReader, "review-guest-connected")

	// A Room update replaces the template's descendants. Establish that known
	// baseline before resolving the live button and span Elements.
	h.app.Jaws.Dirty(room)
	hostRefreshCtx, hostRefreshDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer hostRefreshDone()
	if err = hostReader.readUntil(hostRefreshCtx, func(msg wire.WsMsg) bool {
		return msg.What == what.Inner && msg.Jid == hostPanelJID
	}); err != nil {
		t.Fatalf("waiting for judge room baseline: %v", err)
	}
	guestRefreshCtx, guestRefreshDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer guestRefreshDone()
	if err = guestReader.readUntil(guestRefreshCtx, func(msg wire.WsMsg) bool {
		return msg.What == what.Inner && msg.Jid == guestPanelJID
	}); err != nil {
		t.Fatalf("waiting for non-judge room baseline: %v", err)
	}
	drainImmediateModeAlert(t, hostRQ, hostReader, "review-host-baseline", nil)
	drainImmediateModeAlert(t, guestRQ, guestReader, "review-guest-baseline", nil)

	hostElements := hostRQ.GetElements(room.ReviewButton(host))
	if len(hostElements) != 1 {
		t.Fatalf("judge countdown elements = %#v, want one button", hostElements)
	}
	hostButtonJID := hostElements[0].Jid()
	if _, ok := hostElements[0].UI().(*jui.Button); !ok {
		t.Fatalf("judge countdown UI = %T, want *ui.Button", hostElements[0].UI())
	}
	guestElements := guestRQ.GetElements(room.ReviewStatus(guest))
	if len(guestElements) != 1 {
		t.Fatalf("non-judge countdown elements = %#v, want one span", guestElements)
	}
	guestStatusJID := guestElements[0].Jid()
	if _, ok := guestElements[0].UI().(*jui.Span); !ok {
		t.Fatalf("non-judge countdown UI = %T, want *ui.Span", guestElements[0].UI())
	}

	initialButtonText := string(room.ReviewButton(host).JawsGetHTML(nil))
	initialStatusText := room.ReviewStatus(guest).JawsGet(nil)
	buttonText := regexp.MustCompile(`^Next Round \([1-9][0-9]*\)$`)
	statusText := regexp.MustCompile(`^Next round in [1-9][0-9]* seconds?\.$`)
	var hostPanelUpdated bool
	hostObserve := func(msg wire.WsMsg) {
		if msg.What == what.Inner && msg.Jid == hostPanelJID {
			hostPanelUpdated = true
		}
	}
	hostCtx, hostDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer hostDone()
	if err = hostReader.readUntil(hostCtx, func(msg wire.WsMsg) bool {
		hostObserve(msg)
		return msg.What == what.Inner && msg.Jid == hostButtonJID && msg.Data != initialButtonText && buttonText.MatchString(msg.Data)
	}); err != nil {
		t.Fatalf("waiting for judge countdown update: %v", err)
	}
	drainImmediateModeAlert(t, hostRQ, hostReader, "review-host-updated", hostObserve)

	var guestPanelUpdated bool
	guestObserve := func(msg wire.WsMsg) {
		if msg.What == what.Inner && msg.Jid == guestPanelJID {
			guestPanelUpdated = true
		}
	}
	guestCtx, guestDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer guestDone()
	if err = guestReader.readUntil(guestCtx, func(msg wire.WsMsg) bool {
		guestObserve(msg)
		return msg.What == what.Inner && msg.Jid == guestStatusJID && msg.Data != initialStatusText && statusText.MatchString(msg.Data)
	}); err != nil {
		t.Fatalf("waiting for non-judge countdown update: %v", err)
	}
	drainImmediateModeAlert(t, guestRQ, guestReader, "review-guest-updated", guestObserve)

	if hostPanelUpdated || guestPanelUpdated {
		t.Fatalf("countdown tick updated room panels: host=%v guest=%v", hostPanelUpdated, guestPanelUpdated)
	}
}

func TestPrivateToggleInputUpdatesPeerAndLobby(t *testing.T) {
	h := newLiveHarness(t)

	lobbyHTML := h.get(t, "/")
	lobbySession := h.session(t)
	lobbyRQ := immediateModeRequestForHTML(t, lobbySession, lobbyHTML)
	lobbyConn, lobbyCancel := h.connect(t, lobbyHTML)
	defer lobbyCancel()
	lobbyReader := newImmediateModeWireReader(t, lobbyConn)
	syncImmediateModeRequest(t, lobbyConn, lobbyRQ, lobbyReader, "privacy-lobby-connected")

	hostClient := h.newClient(t)
	h.getWithClient(t, hostClient, "/")
	hostSession := h.sessionForClient(t, hostClient)
	host := h.app.player(hostSession, nil)
	h.app.Manager.SetNickname(host, "Bob")
	room, err := h.app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	var lobbyWrapperJID jid.Jid
	createCtx, createDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer createDone()
	if err := lobbyReader.readUntil(createCtx, func(msg wire.WsMsg) bool {
		if msg.What == what.Inner && strings.Contains(msg.Data, room.Code()) {
			lobbyWrapperJID = msg.Jid
			return true
		}
		return false
	}); err != nil {
		t.Fatalf("waiting for public room in lobby: %v", err)
	}
	drainImmediateModeAlert(t, lobbyRQ, lobbyReader, "public-room-visible", nil)

	editorHTML := h.getWithClient(t, hostClient, "/room/"+room.Code())
	editorRQ := immediateModeRequestForHTML(t, hostSession, editorHTML)
	editorCheckboxJID := immediateModePrivateCheckboxJID(t, editorHTML)
	editorConn, editorCancel := h.connectWithClient(t, hostClient, editorHTML)
	defer editorCancel()
	editorReader := newImmediateModeWireReader(t, editorConn)
	syncImmediateModeRequest(t, editorConn, editorRQ, editorReader, "privacy-editor-connected")

	// The originating browser already applied its checked state before sending Input.
	// A second live rendering proves the Binder's &room.private tag reaches peers.
	peerHTML := h.getWithClient(t, hostClient, "/room/"+room.Code())
	peerRQ := immediateModeRequestForHTML(t, hostSession, peerHTML)
	peerCheckboxJID := immediateModePrivateCheckboxJID(t, peerHTML)
	peerConn, peerCancel := h.connectWithClient(t, hostClient, peerHTML)
	defer peerCancel()
	peerReader := newImmediateModeWireReader(t, peerConn)
	syncImmediateModeRequest(t, peerConn, peerRQ, peerReader, "privacy-peer-connected")

	writeCtx, writeDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer writeDone()
	input := wire.WsMsg{Jid: editorCheckboxJID, What: what.Input, Data: "true"}
	if err := editorConn.Write(writeCtx, websocket.MessageText, []byte(input.Format())); err != nil {
		t.Fatalf("Checkbox Input write error = %v", err)
	}

	peerCtx, peerDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer peerDone()
	if err := peerReader.readUntil(peerCtx, func(msg wire.WsMsg) bool {
		return msg.Jid == peerCheckboxJID && msg.What == what.Value && msg.Data == "true"
	}); err != nil {
		t.Fatalf("waiting for peer checkbox Value: %v", err)
	}
	drainImmediateModeAlert(t, peerRQ, peerReader, "private-checkbox-updated", nil)

	var hiddenLobby wire.WsMsg
	hideCtx, hideDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer hideDone()
	if err := lobbyReader.readUntil(hideCtx, func(msg wire.WsMsg) bool {
		if msg.What == what.Inner && strings.Contains(msg.Data, "No rooms yet") {
			hiddenLobby = msg
			return true
		}
		return false
	}); err != nil {
		t.Fatalf("waiting for private room removal from lobby: %v", err)
	}
	drainImmediateModeAlert(t, lobbyRQ, lobbyReader, "private-room-hidden", nil)
	drainImmediateModeAlert(t, editorRQ, editorReader, "privacy-editor-complete", nil)

	if !room.IsPrivate() {
		t.Fatal("real checkbox Input did not make the room private")
	}
	if hiddenLobby.Jid != lobbyWrapperJID {
		t.Fatalf("lobby wrapper Jid changed from %v to %v", lobbyWrapperJID, hiddenLobby.Jid)
	}
	if strings.Contains(hiddenLobby.Data, room.Code()) {
		t.Fatalf("private room remained in lobby update: %s", hiddenLobby.Data)
	}
}

func TestDeckInputScopesRejectedAndAcceptedUpdates(t *testing.T) {
	h := newLiveHarness(t)

	h.get(t, "/")
	hostSession := h.session(t)
	host := h.app.player(hostSession, nil)
	room, err := h.app.Manager.CreateRoom(host, h.app.Catalog.DefaultDecks())
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	guestClient := h.newClient(t)
	h.getWithClient(t, guestClient, "/")
	guestSession := h.sessionForClient(t, guestClient)
	guest := h.app.player(guestSession, nil)
	if _, err = h.app.Manager.JoinRoom(room.Code(), guest); err != nil {
		t.Fatalf("JoinRoom() error = %v", err)
	}

	hostHTML := h.get(t, "/room/"+room.Code())
	hostRQ := immediateModeRequestForHTML(t, hostSession, hostHTML)
	hostPanelJID := immediateModeTemplateJID(t, hostRQ, hostHTML, "room_game_lobby.html")
	hostSummaryJID := immediateModeTemplateJID(t, hostRQ, hostHTML, "room_summary_panel.html")
	guestHTML := h.getWithClient(t, guestClient, "/room/"+room.Code())
	guestRQ := immediateModeRequestForHTML(t, guestSession, guestHTML)
	guestPanelJID := immediateModeTemplateJID(t, guestRQ, guestHTML, "room_game_lobby.html")
	guestSummaryJID := immediateModeTemplateJID(t, guestRQ, guestHTML, "room_summary_panel.html")

	extra := h.app.Catalog.DeckByID("extra")
	deckTag := roomDeckTag{Room: room, Deck: extra}
	hostDeckElements := hostRQ.GetElements(deckTag)
	guestDeckElements := guestRQ.GetElements(deckTag)
	if len(hostDeckElements) != 1 || len(guestDeckElements) != 1 {
		t.Fatalf("extra-deck elements = host %#v, guest %#v; want one each", hostDeckElements, guestDeckElements)
	}
	if hostDeckElements[0].HasTag(room) || guestDeckElements[0].HasTag(room) {
		t.Fatal("deck checkbox retained the broad Room tag")
	}
	hostDeckJID := hostDeckElements[0].Jid()
	guestDeckJID := guestDeckElements[0].Jid()

	hostConn, hostCancel := h.connect(t, hostHTML)
	defer hostCancel()
	hostReader := newImmediateModeWireReader(t, hostConn)
	guestConn, guestCancel := h.connectWithClient(t, guestClient, guestHTML)
	defer guestCancel()
	guestReader := newImmediateModeWireReader(t, guestConn)
	syncImmediateModeRequest(t, hostConn, hostRQ, hostReader, "deck-host-connected")
	syncImmediateModeRequest(t, guestConn, guestRQ, guestReader, "deck-guest-connected")

	rejectedWriteCtx, rejectedWriteDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	rejectedInput := wire.WsMsg{Jid: guestDeckJID, What: what.Input, Data: "true"}
	if err = guestConn.Write(rejectedWriteCtx, websocket.MessageText, []byte(rejectedInput.Format())); err != nil {
		rejectedWriteDone()
		t.Fatalf("guest deck Input write error = %v", err)
	}
	rejectedWriteDone()

	var guestRejected []wire.WsMsg
	rejectedCtx, rejectedDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	defer rejectedDone()
	if err = guestReader.readUntil(rejectedCtx, func(msg wire.WsMsg) bool {
		guestRejected = append(guestRejected, msg)
		return msg.Jid == guestDeckJID && msg.What == what.Value && msg.Data == "false"
	}); err != nil {
		t.Fatalf("waiting for rejected checkbox correction: %v", err)
	}
	drainImmediateModeAlert(t, guestRQ, guestReader, "deck-guest-rejected", func(msg wire.WsMsg) {
		guestRejected = append(guestRejected, msg)
	})
	var hostRejected []wire.WsMsg
	drainImmediateModeAlert(t, hostRQ, hostReader, "deck-host-rejected", func(msg wire.WsMsg) {
		hostRejected = append(hostRejected, msg)
	})
	for _, observed := range []struct {
		name     string
		messages []wire.WsMsg
		panel    jid.Jid
		summary  jid.Jid
	}{
		{name: "host", messages: hostRejected, panel: hostPanelJID, summary: hostSummaryJID},
		{name: "guest", messages: guestRejected, panel: guestPanelJID, summary: guestSummaryJID},
	} {
		for _, msg := range observed.messages {
			if msg.What == what.Inner && (msg.Jid == observed.panel || msg.Jid == observed.summary) {
				t.Fatalf("rejected input updated %s room region %v: %#v", observed.name, msg.Jid, msg)
			}
		}
	}
	if room.DeckEnabled(extra) {
		t.Fatal("rejected guest input enabled the deck")
	}

	acceptedInput := wire.WsMsg{Jid: hostDeckJID, What: what.Input, Data: "true"}
	acceptedWriteCtx, acceptedWriteDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
	if err = hostConn.Write(acceptedWriteCtx, websocket.MessageText, []byte(acceptedInput.Format())); err != nil {
		acceptedWriteDone()
		t.Fatalf("host deck Input write error = %v", err)
	}
	acceptedWriteDone()
	for _, peer := range []struct {
		name   string
		reader immediateModeWireReader
		panel  jid.Jid
	}{
		{name: "host", reader: hostReader, panel: hostPanelJID},
		{name: "guest", reader: guestReader, panel: guestPanelJID},
	} {
		acceptedCtx, acceptedDone := context.WithTimeout(t.Context(), immediateModeTestTimeout)
		if err = peer.reader.readUntil(acceptedCtx, func(msg wire.WsMsg) bool {
			return msg.Jid == peer.panel && msg.What == what.Inner && strings.Contains(msg.Data, "2 black / 4 white selected")
		}); err != nil {
			acceptedDone()
			t.Fatalf("waiting for accepted %s room update: %v", peer.name, err)
		}
		acceptedDone()
	}
	if !room.DeckEnabled(extra) {
		t.Fatal("accepted host input did not enable the deck")
	}
}

func TestSteadyUpdatesRetainTemplateWrappers(t *testing.T) {
	t.Run("manager lobby", func(t *testing.T) {
		h := newLiveHarness(t)
		pageHTML := h.get(t, "/")
		sess := h.session(t)
		conn, cancel := h.connect(t, pageHTML)
		defer cancel()
		reader := newImmediateModeWireReader(t, conn)
		rq := immediateModeRequestForHTML(t, sess, pageHTML)
		syncImmediateModeRequest(t, conn, rq, reader, "lobby-connected")
		initialJIDs := immediateModeHTMLJIDs(pageHTML)

		host := &game.Player{Nickname: "Bob", NicknameInput: "Bob"}
		room, err := h.app.Manager.CreateRoom(host, h.app.Catalog.DefaultDecks())
		if err != nil {
			t.Fatalf("CreateRoom() error = %v", err)
		}
		h.app.Jaws.Dirty(h.app.Manager)

		var target wire.WsMsg
		var mutations []wire.WsMsg
		var unknownInner []jid.Jid
		observe := func(msg wire.WsMsg) {
			if immediateModeContainerMutation(msg) {
				mutations = append(mutations, msg)
			}
			if msg.What == what.Inner && !initialJIDs[msg.Jid] {
				unknownInner = append(unknownInner, msg.Jid)
			}
		}
		ctx, done := context.WithTimeout(t.Context(), immediateModeTestTimeout)
		defer done()
		if err := reader.readUntil(ctx, func(msg wire.WsMsg) bool {
			observe(msg)
			if msg.What == what.Inner && strings.Contains(msg.Data, room.Code()) {
				target = msg
				return true
			}
			return false
		}); err != nil {
			t.Fatalf("waiting for lobby Inner: %v", err)
		}
		drainImmediateModeAlert(t, rq, reader, "lobby-updated", observe)
		if target.Jid <= 0 || !initialJIDs[target.Jid] {
			t.Fatalf("lobby Inner target %v was not an initial wrapper", target.Jid)
		}
		if len(unknownInner) > 0 {
			t.Fatalf("Inner targets not present in initial document: %v", unknownInner)
		}
		if len(mutations) > 0 {
			t.Fatalf("steady lobby update emitted Container mutations: %v", mutations)
		}
	})

	t.Run("seated room", func(t *testing.T) {
		h := newLiveHarness(t)
		_, host := livePlayer(t, h, "Alice")
		room, err := h.app.Manager.CreateRoom(host, h.app.Catalog.DefaultDecks())
		if err != nil {
			t.Fatalf("CreateRoom() error = %v", err)
		}
		pageHTML := h.get(t, "/room/"+room.Code())
		sess := h.session(t)
		conn, cancel := h.connect(t, pageHTML)
		defer cancel()
		reader := newImmediateModeWireReader(t, conn)
		rq := immediateModeRequestForHTML(t, sess, pageHTML)
		syncImmediateModeRequest(t, conn, rq, reader, "room-connected")
		initialJIDs := immediateModeHTMLJIDs(pageHTML)

		if _, err := room.SetDeckEnabled(host, h.app.Catalog.DeckByID("extra"), true); err != nil {
			t.Fatalf("SetDeckEnabled() error = %v", err)
		}
		h.app.Jaws.Dirty(room)

		var target wire.WsMsg
		var mutations []wire.WsMsg
		var unknownInner []jid.Jid
		observe := func(msg wire.WsMsg) {
			if immediateModeContainerMutation(msg) {
				mutations = append(mutations, msg)
			}
			if msg.What == what.Inner && !initialJIDs[msg.Jid] {
				unknownInner = append(unknownInner, msg.Jid)
			}
		}
		ctx, done := context.WithTimeout(t.Context(), immediateModeTestTimeout)
		defer done()
		if err := reader.readUntil(ctx, func(msg wire.WsMsg) bool {
			observe(msg)
			if msg.What == what.Inner && strings.Contains(msg.Data, "2 black / 4 white selected") {
				target = msg
				return true
			}
			return false
		}); err != nil {
			t.Fatalf("waiting for room Inner: %v", err)
		}
		drainImmediateModeAlert(t, rq, reader, "room-updated", observe)
		if target.Jid <= 0 || !initialJIDs[target.Jid] {
			t.Fatalf("room Inner target %v was not an initial wrapper", target.Jid)
		}
		if len(unknownInner) > 0 {
			t.Fatalf("Inner targets not present in initial document: %v", unknownInner)
		}
		if len(mutations) > 0 {
			t.Fatalf("steady room update emitted Container mutations: %v", mutations)
		}
	})
}

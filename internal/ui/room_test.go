package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/bind"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/jaws/lib/what"
	"github.com/linkdata/jaws/lib/wire"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

func TestApplyCardSelectionReplacesSinglePickSelection(t *testing.T) {
	w1 := &deck.WhiteCard{ID: "w1"}
	w2 := &deck.WhiteCard{ID: "w2"}
	player := &game.Player{SelectedCards: []*deck.WhiteCard{w1}}

	changed := applyCardSelection(player, w2, 1)

	if len(player.SelectedCards) != 1 || player.SelectedCards[0] != w2 {
		t.Fatalf("SelectedCards = %v, want [w2]", player.SelectedCards)
	}
	if !changed {
		t.Fatalf("applyCardSelection() = (%v), want (true)", changed)
	}
}

func TestApplyCardSelectionKeepsMultiPickLimit(t *testing.T) {
	w1 := &deck.WhiteCard{ID: "w1"}
	w2 := &deck.WhiteCard{ID: "w2"}
	w3 := &deck.WhiteCard{ID: "w3"}
	player := &game.Player{SelectedCards: []*deck.WhiteCard{w1, w2}}

	changed := applyCardSelection(player, w3, 2)

	if len(player.SelectedCards) != 2 || player.SelectedCards[0] != w1 || player.SelectedCards[1] != w2 {
		t.Fatalf("SelectedCards = %v, want unchanged", player.SelectedCards)
	}
	if changed {
		t.Fatalf("applyCardSelection() = (%v), want no mutation", changed)
	}
}

func TestRoomScoreTargetSliderRespectsPermissions(t *testing.T) {
	app, mux := testPlayableApp(t)
	handler := app.Middleware(mux)

	hostSess := newTestSession(t, app, handler)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestSess := newTestSession(t, app, handler)
	guest := app.player(guestSess, nil)
	app.setNickname(guest, "Bob")
	if _, err := app.joinRoom(guest, room.Code()); err != nil {
		t.Fatalf("joinRoom() error = %v", err)
	}

	guestSlider := room.ScoreTargetSlider(guest)
	if err := guestSlider.JawsSet(newScoreTargetElement(app, guestSlider), 8); err != game.ErrOnlyHostCanEdit {
		t.Fatalf("guestSlider.JawsSet() error = %v, want %v", err, game.ErrOnlyHostCanEdit)
	}
	if got := room.TargetScore(); got != game.ScoreGoal {
		t.Fatalf("TargetScore after non-host set = %d, want %d", got, game.ScoreGoal)
	}

	hostSlider := room.ScoreTargetSlider(host)
	if err := hostSlider.JawsSet(newScoreTargetElement(app, hostSlider), 8); err != nil {
		t.Fatalf("hostSlider.JawsSet() error = %v", err)
	}
	if got := room.TargetScore(); got != 8 {
		t.Fatalf("TargetScore after host set = %d, want 8", got)
	}

	if err := room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	lockedSlider := room.ScoreTargetSlider(host)
	if err := lockedSlider.JawsSet(newScoreTargetElement(app, lockedSlider), 10); err != game.ErrGameInProgress {
		t.Fatalf("lockedSlider.JawsSet() error = %v, want %v", err, game.ErrGameInProgress)
	}
	if got := room.TargetScore(); got != 8 {
		t.Fatalf("TargetScore after in-game set = %d, want 8", got)
	}
}

func TestRoomScoreTargetSliderAllowsOneInDebug(t *testing.T) {
	app, mux := testPlayableAppWithOptions(t, game.Options{MinPlayers: 2, Debug: true})
	handler := app.Middleware(mux)

	hostSess := newTestSession(t, app, handler)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	slider := room.ScoreTargetSlider(host)
	if err := slider.JawsSet(newScoreTargetElement(app, slider), 1); err != nil {
		t.Fatalf("slider.JawsSet() error = %v", err)
	}
	if got := room.TargetScore(); got != 1 {
		t.Fatalf("TargetScore() = %d, want 1", got)
	}

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	roomReq.SetPathValue("code", room.Code())
	roomReq.AddCookie(hostSess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	if body := roomRec.Body.String(); !strings.Contains(body, `min="1"`) {
		t.Fatalf("expected debug score slider min=1, got body %s", body)
	}
}

func TestRoomReceivesLiveTargetScoreUpdates(t *testing.T) {
	h := newLiveHarness(t)

	h.get(t, "/")
	sess := h.session(t)
	player := h.app.player(sess, nil)
	h.app.setNickname(player, "Alice")
	room, err := h.app.createRoom(player)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	html := h.get(t, "/room/"+room.Code())
	conn, cancel := h.connect(t, html)
	defer cancel()

	slider := room.ScoreTargetSlider(player)
	if err := slider.JawsSet(newScoreTargetElement(h.app, slider), 7); err != nil {
		t.Fatalf("slider.JawsSet() error = %v", err)
	}

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	if err := readUntilScoreTargetUpdate(ctx, conn, "7"); err != nil {
		t.Fatalf("readUntilScoreTargetUpdate() error = %v", err)
	}
	if got := room.TargetScore(); got != 7 {
		t.Fatalf("TargetScore = %d, want 7", got)
	}
}

func newScoreTargetElement(app *App, slider bind.Binder[int]) (result *jaws.Element) {
	result = app.Jaws.NewRequest(nil).NewElement(jui.NewRange(bind.MakeSetterFloat64(slider)))
	return
}

func newTestSession(t *testing.T, app *App, handler http.Handler) (result *jaws.Session) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	result = app.Jaws.GetSession(req)
	if result == nil {
		t.Fatal("expected JaWS session")
	}
	return
}

func readUntilScoreTargetUpdate(ctx context.Context, conn *websocket.Conn, want string) (errResult error) {
	for {
		_, body, err := conn.Read(ctx)
		if err != nil {
			errResult = err
			return
		}
		text := string(body)
		if strings.Contains(text, `value="`+want+`"`) || strings.Contains(text, `>`+want+`<`) {
			return
		}
		for _, line := range strings.Split(text, "\n") {
			if line == "" {
				continue
			}
			msg, ok := wire.Parse([]byte(line + "\n"))
			if !ok {
				continue
			}
			switch msg.What {
			case what.Value, what.Inner:
				if msg.Data == want {
					return
				}
			case what.Append, what.Replace:
				if strings.Contains(msg.Data, `value="`+want+`"`) || strings.Contains(msg.Data, `>`+want+`<`) {
					return
				}
			}
		}
	}
}

func testPlayableApp(t *testing.T) (result1 *App, result2 *http.ServeMux) {
	result1, result2 = testPlayableAppWithOptions(t, game.Options{MinPlayers: 2})
	return
}

func testPlayableAppWithOptions(t *testing.T, opts game.Options) (result1 *App, result2 *http.ServeMux) {
	t.Helper()

	jw, err := jaws.New()
	if err != nil {
		t.Fatalf("jaws.New() error = %v", err)
	}
	t.Cleanup(jw.Close)
	go jw.Serve()

	catalog := testPlayableCatalog(t)
	app := New(jw, catalog, game.NewManagerWithOptions(catalog, opts))
	mux := http.NewServeMux()
	if err := app.SetupRoutes(mux); err != nil {
		t.Fatalf("SetupRoutes() error = %v", err)
	}
	result1, result2 = app, mux
	return
}

func testPlayableCatalog(t *testing.T) (result *deck.Catalog) {
	t.Helper()

	fsys := fstest.MapFS{
		"assets/decks/base/deck.json": {Data: []byte(`{"id":"base","name":"Base","enabled_by_default":true}`)},
	}
	blackIDs := make([]string, 0, 50)
	whiteIDs := make([]string, 0, 80)
	for i := 1; i <= 50; i++ {
		id := fmt.Sprintf("b%02d", i)
		blackIDs = append(blackIDs, id)
		fsys["assets/cards/black/"+id+".json"] = &fstest.MapFile{
			Data: []byte(fmt.Sprintf(`{"id":"%s","text":"Black card %d?"}`, id, i)),
		}
	}
	for i := 1; i <= 80; i++ {
		id := fmt.Sprintf("w%02d", i)
		whiteIDs = append(whiteIDs, id)
		fsys["assets/cards/white/"+id+".json"] = &fstest.MapFile{
			Data: []byte(fmt.Sprintf(`{"id":"%s","text":"White card %d"}`, id, i)),
		}
	}
	blackJSON, err := json.Marshal(blackIDs)
	if err != nil {
		t.Fatalf("json.Marshal(blackIDs) error = %v", err)
	}
	whiteJSON, err := json.Marshal(whiteIDs)
	if err != nil {
		t.Fatalf("json.Marshal(whiteIDs) error = %v", err)
	}
	fsys["assets/decks/base/black.json"] = &fstest.MapFile{Data: blackJSON}
	fsys["assets/decks/base/white.json"] = &fstest.MapFile{Data: whiteJSON}

	result, err = deck.LoadFS(fsys)
	if err != nil {
		t.Fatalf("LoadFS() error = %v", err)
	}
	return
}

func TestRoomRendersExistingRoom(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	sess := app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected JaWS session")
	}
	player := app.player(sess, req)
	app.setNickname(player, "Alice")
	room, err := app.createRoom(player)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	roomReq.SetPathValue("code", room.Code())
	roomReq.AddCookie(sess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	body := roomRec.Body.String()
	if !strings.Contains(body, room.Code()) || !strings.Contains(body, "Card Packs") {
		t.Fatalf("unexpected room body: %s", body)
	}
	if !strings.Contains(body, "Private game") || !strings.Contains(body, "private-game-group") {
		t.Fatalf("expected lobby controls to include private-game input group: %s", body)
	}
	privateToggle := regexp.MustCompile(`<input[^>]*class="form-check-input private-toggle-checkbox mt-0 me-1"[^>]*>`)
	match := privateToggle.FindString(body)
	if match == "" || strings.Contains(match, `checked`) {
		t.Fatalf("expected private checkbox to render unchecked by default, got %q", match)
	}
	if !(strings.Contains(body, "Target score") && strings.Contains(body, "Start Game")) {
		t.Fatalf("expected unified lobby controls to include target score and start button: %s", body)
	}
	if !strings.Contains(body, `row row-cols-1 row-cols-md-3 g-2`) {
		t.Fatalf("expected deck selection grid to render three columns at the normal breakpoint: %s", body)
	}
	if !strings.Contains(body, `data-bs-target="#nicknameModal"`) || !strings.Contains(body, `id="nicknameModal"`) {
		t.Fatalf("expected room body to include nickname modal trigger and dialog: %s", body)
	}
}

func TestRoomAutoJoinsLobbyRoom(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	hostReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	hostRec := httptest.NewRecorder()
	handler.ServeHTTP(hostRec, hostReq)
	hostSess := app.Jaws.GetSession(hostReq)
	if hostSess == nil {
		t.Fatal("expected JaWS session")
	}
	host := app.player(hostSess, hostReq)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	joinReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	joinRec := httptest.NewRecorder()
	handler.ServeHTTP(joinRec, joinReq)
	joinSess := app.Jaws.GetSession(joinReq)
	if joinSess == nil {
		t.Fatal("expected JaWS session")
	}
	guest := app.player(joinSess, joinReq)
	app.setNickname(guest, "Bob")

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	roomReq.SetPathValue("code", room.Code())
	roomReq.AddCookie(joinSess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	if guest.Room != room {
		t.Fatal("expected guest to auto-join lobby room")
	}
	body := roomRec.Body.String()
	if !strings.Contains(body, "Card Packs") {
		t.Fatalf("expected joined room body, got %s", body)
	}
}

func TestPrivateRoomStillAutoJoinsByDirectURL(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	hostReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	hostRec := httptest.NewRecorder()
	handler.ServeHTTP(hostRec, hostReq)
	hostSess := app.Jaws.GetSession(hostReq)
	if hostSess == nil {
		t.Fatal("expected JaWS session")
	}
	host := app.player(hostSess, hostReq)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}
	if err := room.SetPrivate(host, true); err != nil {
		t.Fatalf("SetPrivate() error = %v", err)
	}

	joinReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	joinRec := httptest.NewRecorder()
	handler.ServeHTTP(joinRec, joinReq)
	joinSess := app.Jaws.GetSession(joinReq)
	if joinSess == nil {
		t.Fatal("expected JaWS session")
	}
	guest := app.player(joinSess, joinReq)
	app.setNickname(guest, "Bob")

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	roomReq.SetPathValue("code", room.Code())
	roomReq.AddCookie(joinSess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	if guest.Room != room {
		t.Fatal("expected guest to auto-join private room by direct URL")
	}
	if body := roomRec.Body.String(); !strings.Contains(body, "Card Packs") {
		t.Fatalf("expected private room body, got %s", body)
	}
}

func TestRoomAutoJoinsGameInProgress(t *testing.T) {
	app, mux := testPlayableApp(t)
	handler := app.Middleware(mux)

	hostReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	hostRec := httptest.NewRecorder()
	handler.ServeHTTP(hostRec, hostReq)
	hostSess := app.Jaws.GetSession(hostReq)
	if hostSess == nil {
		t.Fatal("expected JaWS session")
	}
	host := app.player(hostSess, hostReq)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guest1Sess := newTestSession(t, app, handler)
	guest1 := app.player(guest1Sess, nil)
	app.setNickname(guest1, "Bob")
	if _, err := app.joinRoom(guest1, room.Code()); err != nil {
		t.Fatalf("JoinRoom(guest1) error = %v", err)
	}
	if err := room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	joinReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	joinRec := httptest.NewRecorder()
	handler.ServeHTTP(joinRec, joinReq)
	joinSess := app.Jaws.GetSession(joinReq)
	if joinSess == nil {
		t.Fatal("expected JaWS session")
	}
	guest := app.player(joinSess, joinReq)
	app.setNickname(guest, "Drew")

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	roomReq.SetPathValue("code", room.Code())
	roomReq.AddCookie(joinSess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	if guest.Room != room {
		t.Fatal("expected guest to auto-join game in progress")
	}
	body := roomRec.Body.String()
	if !strings.Contains(body, "Your Hand") {
		t.Fatalf("expected in-progress room body for joined player, got %s", body)
	}
	if strings.Contains(body, `<button class="card-face card-face-white`) {
		t.Fatalf("expected hand cards to render as clickable template elements instead of buttons, got %s", body)
	}
}

func TestHandCardTemplateDispatchesClickToSelectionHandler(t *testing.T) {
	app, mux := testPlayableApp(t)
	handler := app.Middleware(mux)

	hostSess := newTestSession(t, app, handler)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestSess := newTestSession(t, app, handler)
	guest := app.player(guestSess, nil)
	app.setNickname(guest, "Bob")
	if _, err := app.joinRoom(guest, room.Code()); err != nil {
		t.Fatalf("joinRoom() error = %v", err)
	}
	if err := room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !room.CanSubmit(guest) {
		t.Fatal("expected guest to be able to submit in opening round")
	}

	hand := room.HandFor(guest)
	if len(hand) == 0 {
		t.Fatal("expected non-empty hand")
	}
	card := hand[0]

	dot := templateDot{App: app, Player: guest, Room: room}
	view := dot.HandCardView(card)

	req := app.Jaws.NewRequest(nil)
	elem := req.NewElement(jui.Template{Name: "hand_card_clickable.html", Dot: view})
	var rendered bytes.Buffer
	if err := elem.JawsRender(&rendered, []any{dot.CardAttrs(), dot.CardClass(card)}); err != nil {
		t.Fatalf("JawsRender() error = %v", err)
	}
	html := rendered.String()
	if !strings.Contains(html, `<div id="Jid.`) || !strings.Contains(html, `role="button"`) {
		t.Fatalf("expected clickable template wrapper div, got %s", html)
	}
	if strings.Contains(html, "<button") {
		t.Fatalf("expected non-button hand card template rendering, got %s", html)
	}

	if err := jaws.CallEventHandlers(elem.Ui(), elem, what.Click, "ignored"); err != nil {
		t.Fatalf("CallEventHandlers(first click) error = %v", err)
	}
	if len(guest.SelectedCards) != 1 || guest.SelectedCards[0] != card {
		t.Fatalf("SelectedCards after first click = %#v, want [%#v]", guest.SelectedCards, card)
	}

	if err := jaws.CallEventHandlers(elem.Ui(), elem, what.Click, "ignored"); err != nil {
		t.Fatalf("CallEventHandlers(second click) error = %v", err)
	}
	if len(guest.SelectedCards) != 0 {
		t.Fatalf("SelectedCards after second click = %#v, want empty", guest.SelectedCards)
	}
}

func TestSubmissionTemplateDispatchesClickToSelectionHandler(t *testing.T) {
	app, mux := testPlayableApp(t)
	handler := app.Middleware(mux)

	hostSess := newTestSession(t, app, handler)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestSess := newTestSession(t, app, handler)
	guest := app.player(guestSess, nil)
	app.setNickname(guest, "Bob")
	if _, err := app.joinRoom(guest, room.Code()); err != nil {
		t.Fatalf("joinRoom() error = %v", err)
	}
	if err := room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !room.IsJudge(host) {
		t.Fatal("expected host to be judge for the opening round")
	}

	hand := room.HandFor(guest)
	if len(hand) == 0 {
		t.Fatal("expected non-empty hand")
	}
	if err := room.PlayCards(guest, []*deck.WhiteCard{hand[0]}); err != nil {
		t.Fatalf("PlayCards() error = %v", err)
	}
	if room.State() != game.StateJudging {
		t.Fatalf("State() = %s, want %s", room.State(), game.StateJudging)
	}
	submissions := room.Submissions()
	if len(submissions) == 0 {
		t.Fatal("expected at least one submission")
	}
	submission := submissions[0]

	dot := templateDot{App: app, Player: host, Room: room}
	view := dot.SubmissionView(submission)

	req := app.Jaws.NewRequest(nil)
	elem := req.NewElement(jui.Template{Name: "submission_clickable.html", Dot: view})
	var rendered bytes.Buffer
	if err := elem.JawsRender(&rendered, []any{dot.SubmissionAttrs(), dot.SubmissionClass(submission)}); err != nil {
		t.Fatalf("JawsRender() error = %v", err)
	}
	html := rendered.String()
	if !strings.Contains(html, `<div id="Jid.`) || !strings.Contains(html, `role="button"`) {
		t.Fatalf("expected clickable template wrapper div, got %s", html)
	}
	if strings.Contains(html, "<button") {
		t.Fatalf("expected non-button submission template rendering, got %s", html)
	}

	if err := jaws.CallEventHandlers(elem.Ui(), elem, what.Click, "ignored"); err != nil {
		t.Fatalf("CallEventHandlers(first click) error = %v", err)
	}
	if host.SelectedSubmission != submission {
		t.Fatalf("SelectedSubmission after first click = %#v, want %#v", host.SelectedSubmission, submission)
	}

	if err := jaws.CallEventHandlers(elem.Ui(), elem, what.Click, "ignored"); err != nil {
		t.Fatalf("CallEventHandlers(second click) error = %v", err)
	}
	if host.SelectedSubmission != nil {
		t.Fatalf("SelectedSubmission after second click = %#v, want nil", host.SelectedSubmission)
	}
}

func TestRoomShowsJudgingSubmissionsToNonJudge(t *testing.T) {
	app, mux := testPlayableApp(t)
	handler := app.Middleware(mux)

	hostSess := newTestSession(t, app, handler)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestSess := newTestSession(t, app, handler)
	guest := app.player(guestSess, nil)
	app.setNickname(guest, "Bob")
	if _, err := app.joinRoom(guest, room.Code()); err != nil {
		t.Fatalf("joinRoom() error = %v", err)
	}
	if err := room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !room.IsJudge(host) {
		t.Fatal("expected host to be judge for the opening round")
	}

	hand := room.HandFor(guest)
	if err := room.PlayCards(guest, []*deck.WhiteCard{hand[0]}); err != nil {
		t.Fatalf("PlayCards() error = %v", err)
	}
	if room.State() != game.StateJudging {
		t.Fatalf("State() = %s, want %s", room.State(), game.StateJudging)
	}

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	roomReq.SetPathValue("code", room.Code())
	roomReq.AddCookie(guestSess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}

	body := roomRec.Body.String()
	if !strings.Contains(body, "Alice is picking the winner") {
		t.Fatalf("expected non-judge judging view to show waiting title, got %s", body)
	}
	if !strings.Contains(body, "card-face card-face-white") || !strings.Contains(body, "White card") {
		t.Fatalf("expected non-judge judging view to show submitted card sets, got %s", body)
	}
	if strings.Contains(body, ">Pick Winner<") {
		t.Fatalf("did not expect non-judge judging view to render the pick button, got %s", body)
	}
}

func TestRoomShowsRoundWinnerReviewState(t *testing.T) {
	app, mux := testPlayableApp(t)
	handler := app.Middleware(mux)

	hostSess := newTestSession(t, app, handler)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestSess := newTestSession(t, app, handler)
	guest := app.player(guestSess, nil)
	app.setNickname(guest, "Bob")
	if _, err := app.joinRoom(guest, room.Code()); err != nil {
		t.Fatalf("joinRoom() error = %v", err)
	}
	if err := room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !room.IsJudge(host) {
		t.Fatal("expected host to be judge for the opening round")
	}

	hand := room.HandFor(guest)
	if err := room.PlayCards(guest, []*deck.WhiteCard{hand[0]}); err != nil {
		t.Fatalf("PlayCards() error = %v", err)
	}
	if room.State() != game.StateJudging {
		t.Fatalf("State() = %s, want %s", room.State(), game.StateJudging)
	}
	if err := room.Judge(host, room.Submissions()[0]); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if room.State() != game.StateReview {
		t.Fatalf("State() = %s, want %s", room.State(), game.StateReview)
	}

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
	roomReq.SetPathValue("code", room.Code())
	roomReq.AddCookie(hostSess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}

	body := roomRec.Body.String()
	if !strings.Contains(body, "Bob won the round!") {
		t.Fatalf("expected round winner title, got %s", body)
	}
	if !strings.Contains(body, "review-countdown-button") || !strings.Contains(body, "data-review-deadline=") {
		t.Fatalf("expected review proceed button with countdown data, got %s", body)
	}
	if !strings.Contains(body, "room-player-winner") || !strings.Contains(body, "winner</span>") {
		t.Fatalf("expected sidebar winner highlight, got %s", body)
	}
	if !strings.Contains(body, "is-winning") {
		t.Fatalf("expected winning submission highlight, got %s", body)
	}
}

func TestMissingRoomRendersMissingPanel(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	sess := app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected JaWS session")
	}

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/MISSING", nil)
	roomReq.SetPathValue("code", "MISSING")
	roomReq.AddCookie(sess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)
	if roomRec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
	}
	body := roomRec.Body.String()
	if !strings.Contains(body, "Room not found") {
		t.Fatalf("expected missing-room panel text: %s", body)
	}
}

func TestRoomRedirectsToCurrentRoom(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	sess := app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected JaWS session")
	}
	player := app.player(sess, req)
	app.setNickname(player, "Alice")
	room, err := app.createRoom(player)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	other := &game.Player{Nickname: "Bob", NicknameInput: "Bob"}
	otherRoom, err := app.Manager.CreateRoom(other, app.Catalog.DefaultDeckIDs())
	if err != nil {
		t.Fatalf("CreateRoom(other) error = %v", err)
	}

	roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+otherRoom.Code(), nil)
	roomReq.SetPathValue("code", otherRoom.Code())
	roomReq.AddCookie(sess.Cookie())
	roomRec := httptest.NewRecorder()
	handler.ServeHTTP(roomRec, roomReq)

	if roomRec.Code != http.StatusSeeOther {
		t.Fatalf("ServeHTTP() status = %d, want %d", roomRec.Code, http.StatusSeeOther)
	}
	if got := roomRec.Header().Get("Location"); got != "/room/"+room.Code() {
		t.Fatalf("Location = %q, want %q", got, "/room/"+room.Code())
	}
}

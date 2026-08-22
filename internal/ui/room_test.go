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

func TestRoomTargetScoreBinderRespectsPermissions(t *testing.T) {
	app, _ := testPlayableApp(t)

	hostSess := newTestSession(t, app)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestSess := newTestSession(t, app)
	guest := app.player(guestSess, nil)
	app.setNickname(guest, "Bob")
	if _, err := app.joinRoom(guest, room.Code()); err != nil {
		t.Fatalf("joinRoom() error = %v", err)
	}

	guestSlider := room.TargetScoreBinder(guest)
	if err := guestSlider.JawsSet(newScoreTargetElement(app, guestSlider), 8); err != game.ErrOnlyHostCanEdit {
		t.Fatalf("guestSlider.JawsSet() error = %v, want %v", err, game.ErrOnlyHostCanEdit)
	}
	if got := room.TargetScore(); got != game.ScoreGoal {
		t.Fatalf("TargetScore after non-host set = %d, want %d", got, game.ScoreGoal)
	}

	hostSlider := room.TargetScoreBinder(host)
	if err := hostSlider.JawsSet(newScoreTargetElement(app, hostSlider), 8); err != nil {
		t.Fatalf("hostSlider.JawsSet() error = %v", err)
	}
	if got := room.TargetScore(); got != 8 {
		t.Fatalf("TargetScore after host set = %d, want 8", got)
	}

	if err := room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	lockedSlider := room.TargetScoreBinder(host)
	if err := lockedSlider.JawsSet(newScoreTargetElement(app, lockedSlider), 10); err != game.ErrGameInProgress {
		t.Fatalf("lockedSlider.JawsSet() error = %v, want %v", err, game.ErrGameInProgress)
	}
	if got := room.TargetScore(); got != 8 {
		t.Fatalf("TargetScore after in-game set = %d, want 8", got)
	}
}

func TestRoomTargetScoreBinderAllowsOneInDebug(t *testing.T) {
	app, mux := testPlayableAppWithOptions(t, game.Options{MinPlayers: 2, Debug: true})
	handler := app.Middleware(mux)

	hostSess := newTestSession(t, app)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	slider := room.TargetScoreBinder(host)
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

	slider := room.TargetScoreBinder(player)
	if err := slider.JawsSet(newScoreTargetElement(h.app, slider), 7); err != nil {
		t.Fatalf("slider.JawsSet() error = %v", err)
	}

	ctx, done := context.WithTimeout(t.Context(), 5*time.Second)
	defer done()
	if err := readUntilScoreTargetUpdate(ctx, conn, "7"); err != nil {
		t.Fatalf("readUntilScoreTargetUpdate() error = %v", err)
	}
	if got := room.TargetScore(); got != 7 {
		t.Fatalf("TargetScore = %d, want 7", got)
	}
}

func TestDeckInputReadsWritesAndTagsRoom(t *testing.T) {
	app, _ := testPlayableApp(t)

	hostSess := newTestSession(t, app)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	base := app.Catalog.DeckByID("base")
	input := deckInput{Room: room, Player: host, Deck: base}
	elem := app.Jaws.NewRequest(httptest.NewRecorder(), nil).NewElement(jui.NewCheckbox(input))
	if got := input.JawsGet(elem); !got {
		t.Fatalf("JawsGet() = %v, want selected", got)
	}
	if got := input.JawsGetTag(); got != room {
		t.Fatalf("JawsGetTag() = %v, want Room", got)
	}
	if err := input.JawsSet(elem, false); err != nil {
		t.Fatalf("JawsSet(false) error = %v", err)
	}
	if got := input.JawsGet(elem); got {
		t.Fatalf("JawsGet() after disable = %v, want unselected", got)
	}
	if err := input.JawsSet(elem, true); err != nil {
		t.Fatalf("JawsSet(true) error = %v", err)
	}
	if got := input.JawsGet(elem); !got {
		t.Fatalf("JawsGet() after enable = %v, want selected", got)
	}
	if got := (deckInput{}).JawsGetTag(); got != nil {
		t.Fatalf("zero deckInput JawsGetTag() = %v, want nil", got)
	}
}

func newScoreTargetElement(app *App, slider bind.Binder[int]) (result *jaws.Element) {
	result = app.Jaws.NewRequest(httptest.NewRecorder(), nil).NewElement(jui.NewRange(slider))
	return
}

func newTestSession(t *testing.T, app *App) (result *jaws.Session) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	result = app.Jaws.NewSession(rec, req)
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
	sess := newTestSession(t, app)
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
	hostSess := newTestSession(t, app)
	host := app.player(hostSess, hostReq)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	joinReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	joinSess := newTestSession(t, app)
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
	if guest.Room() != room {
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
	hostSess := newTestSession(t, app)
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
	joinSess := newTestSession(t, app)
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
	if guest.Room() != room {
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
	hostSess := newTestSession(t, app)
	host := app.player(hostSess, hostReq)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guest1Sess := newTestSession(t, app)
	guest1 := app.player(guest1Sess, nil)
	app.setNickname(guest1, "Bob")
	if _, err := app.joinRoom(guest1, room.Code()); err != nil {
		t.Fatalf("JoinRoom(guest1) error = %v", err)
	}
	if err := room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	joinReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	joinSess := newTestSession(t, app)
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
	if guest.Room() != room {
		t.Fatal("expected guest to auto-join game in progress")
	}
	body := roomRec.Body.String()
	if !strings.Contains(body, "Your Hand") {
		t.Fatalf("expected in-progress room body for joined player, got %s", body)
	}
	if strings.Contains(body, `<button class="card-face card-face-white`) {
		t.Fatalf("expected hand cards to render as clickable template elements instead of buttons, got %s", body)
	}
	hasHandCard := false
	for _, tag := range regexp.MustCompile(`<div\b[^>]*>`).FindAllString(body, -1) {
		if strings.Contains(tag, `data-jawstemplate`) &&
			strings.Contains(tag, `class="card-face card-face-white`) &&
			strings.Contains(tag, `role="button"`) {
			hasHandCard = true
			break
		}
	}
	if !hasHandCard || !strings.Contains(body, "White card") {
		t.Fatalf("expected hand cards to render as clickable template elements, got %s", body)
	}
}

func TestHandCardTemplateDispatchesClickToSelectionHandler(t *testing.T) {
	app, _ := testPlayableApp(t)

	hostSess := newTestSession(t, app)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestSess := newTestSession(t, app)
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

	view := whiteCardView{
		Room:           room,
		Player:         guest,
		Card:           card,
		SelectionOrder: room.SelectionOrderFor(guest, card),
		Enabled:        true,
	}

	req := app.Jaws.NewRequest(httptest.NewRecorder(), nil)
	elem := req.NewElement(jui.NewTemplate("div", "hand_card_clickable.html", view))
	var rendered bytes.Buffer
	if err := elem.JawsRender(&rendered, []any{`data-jawstemplate`, `role="button"`, `tabindex="0"`}); err != nil {
		t.Fatalf("JawsRender() error = %v", err)
	}
	html := rendered.String()
	if !strings.Contains(html, `<div id="Jid.`) || !strings.Contains(html, `role="button"`) {
		t.Fatalf("expected clickable template wrapper div, got %s", html)
	}
	if strings.Contains(html, "<button") {
		t.Fatalf("expected non-button hand card template rendering, got %s", html)
	}

	clickData := jaws.Click{Name: "ignored"}.String()
	if err := jaws.CallEventHandlers(elem.UI(), elem, what.Click, clickData); err != nil {
		t.Fatalf("CallEventHandlers(first click) error = %v", err)
	}
	if len(guest.SelectedCards) != 1 || guest.SelectedCards[0] != card {
		t.Fatalf("SelectedCards after first click = %#v, want [%#v]", guest.SelectedCards, card)
	}

	if err := jaws.CallEventHandlers(elem.UI(), elem, what.Click, clickData); err != nil {
		t.Fatalf("CallEventHandlers(second click) error = %v", err)
	}
	if len(guest.SelectedCards) != 0 {
		t.Fatalf("SelectedCards after second click = %#v, want empty", guest.SelectedCards)
	}
}

func TestWhiteCardViewInitialHTMLAttr(t *testing.T) {
	app, _ := testPlayableApp(t)

	hostSess := newTestSession(t, app)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestSess := newTestSession(t, app)
	guest := app.player(guestSess, nil)
	app.setNickname(guest, "Bob")
	if _, err := app.joinRoom(guest, room.Code()); err != nil {
		t.Fatalf("joinRoom() error = %v", err)
	}
	if err := room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	card := room.HandFor(guest)[0]
	view := whiteCardView{Room: room, Player: guest, Card: card, Enabled: true}
	attr := string(view.JawsInitialHTMLAttr(new(jaws.Element)))
	if !strings.Contains(attr, `class="card-face card-face-white w-100 text-start"`) ||
		strings.Contains(attr, "is-selected") || strings.Contains(attr, "disabled") {
		t.Fatalf("initial attributes = %q, want enabled unselected card", attr)
	}

	if !room.ToggleCardSelection(guest, card) {
		t.Fatal("ToggleCardSelection() did not select card")
	}
	selected := view
	selected.SelectionOrder = room.SelectionOrderFor(guest, card)
	if !room.ToggleCardSelection(guest, card) {
		t.Fatal("ToggleCardSelection() did not clear the captured selection")
	}
	attr = string(selected.JawsInitialHTMLAttr(new(jaws.Element)))
	if !strings.Contains(attr, "is-selected") || strings.Contains(attr, "disabled") {
		t.Fatalf("captured selected attributes = %q, want enabled selected card", attr)
	}

	blocked := view
	blocked.Enabled = false
	if attr = string(blocked.JawsInitialHTMLAttr(new(jaws.Element))); !strings.Contains(attr, "disabled") {
		t.Fatalf("disabled snapshot attributes = %q, want disabled", attr)
	}
}

func TestSubmissionTemplateDispatchesClickToSelectionHandler(t *testing.T) {
	app, _ := testPlayableApp(t)

	hostSess := newTestSession(t, app)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestSess := newTestSession(t, app)
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

	view := submissionView{Room: room, Player: host, Submission: submission}

	req := app.Jaws.NewRequest(httptest.NewRecorder(), nil)
	elem := req.NewElement(jui.NewTemplate("div", "submission_clickable.html", view))
	var rendered bytes.Buffer
	if err := elem.JawsRender(&rendered, []any{`data-jawstemplate`, `role="button"`, `tabindex="0"`}); err != nil {
		t.Fatalf("JawsRender() error = %v", err)
	}
	html := rendered.String()
	if !strings.Contains(html, `<div id="Jid.`) || !strings.Contains(html, `role="button"`) {
		t.Fatalf("expected clickable template wrapper div, got %s", html)
	}
	if strings.Contains(html, "<button") {
		t.Fatalf("expected non-button submission template rendering, got %s", html)
	}

	clickData := jaws.Click{Name: "ignored"}.String()
	if err := jaws.CallEventHandlers(elem.UI(), elem, what.Click, clickData); err != nil {
		t.Fatalf("CallEventHandlers(first click) error = %v", err)
	}
	if host.SelectedSubmission != submission {
		t.Fatalf("SelectedSubmission after first click = %#v, want %#v", host.SelectedSubmission, submission)
	}

	if err := jaws.CallEventHandlers(elem.UI(), elem, what.Click, clickData); err != nil {
		t.Fatalf("CallEventHandlers(second click) error = %v", err)
	}
	if host.SelectedSubmission != nil {
		t.Fatalf("SelectedSubmission after second click = %#v, want nil", host.SelectedSubmission)
	}
}

func TestSubmissionViewInitialHTMLAttr(t *testing.T) {
	app, _ := testPlayableApp(t)

	hostSess := newTestSession(t, app)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestSess := newTestSession(t, app)
	guest := app.player(guestSess, nil)
	app.setNickname(guest, "Bob")
	if _, err := app.joinRoom(guest, room.Code()); err != nil {
		t.Fatalf("joinRoom() error = %v", err)
	}
	if err := room.Start(host); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	hand := room.HandFor(guest)
	if err := room.PlayCards(guest, []*deck.WhiteCard{hand[0]}); err != nil {
		t.Fatalf("PlayCards() error = %v", err)
	}
	submission := room.Submissions()[0]
	view := submissionView{Room: room, Player: host, Submission: submission}
	attr := string(view.JawsInitialHTMLAttr(new(jaws.Element)))
	if !strings.Contains(attr, `class="card-face card-face-white w-100 text-start"`) ||
		strings.Contains(attr, "is-selected") || strings.Contains(attr, "is-winning") || strings.Contains(attr, "disabled") {
		t.Fatalf("initial attributes = %q, want enabled unselected submission", attr)
	}

	if !room.ToggleSubmissionSelection(host, submission) {
		t.Fatal("ToggleSubmissionSelection() did not select submission")
	}
	attr = string(view.JawsInitialHTMLAttr(new(jaws.Element)))
	if !strings.Contains(attr, "is-selected") || strings.Contains(attr, "disabled") {
		t.Fatalf("selected attributes = %q, want enabled selected submission", attr)
	}

	if err := room.Judge(host, submission); err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	attr = string(view.JawsInitialHTMLAttr(new(jaws.Element)))
	if !strings.Contains(attr, "is-winning") || !strings.Contains(attr, "disabled") {
		t.Fatalf("review attributes = %q, want disabled winning submission", attr)
	}
}

func TestRoomShowsJudgingSubmissionsToNonJudge(t *testing.T) {
	app, mux := testPlayableApp(t)
	handler := app.Middleware(mux)

	hostSess := newTestSession(t, app)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestSess := newTestSession(t, app)
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

	hostSess := newTestSession(t, app)
	host := app.player(hostSess, nil)
	app.setNickname(host, "Alice")
	room, err := app.createRoom(host)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	guestSess := newTestSession(t, app)
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

	renderRoom := func(sess *jaws.Session) (result string) {
		t.Helper()
		roomReq := httptest.NewRequest(http.MethodGet, "http://example.test/room/"+room.Code(), nil)
		roomReq.SetPathValue("code", room.Code())
		roomReq.AddCookie(sess.Cookie())
		roomRec := httptest.NewRecorder()
		handler.ServeHTTP(roomRec, roomReq)
		if roomRec.Code != http.StatusOK {
			t.Fatalf("ServeHTTP() status = %d", roomRec.Code)
		}
		result = roomRec.Body.String()
		return
	}

	body := renderRoom(hostSess)
	if !strings.Contains(body, "Bob won the round!") {
		t.Fatalf("expected round winner title, got %s", body)
	}
	if !strings.Contains(body, `class="btn btn-primary review-countdown-button"`) {
		t.Fatalf("expected review proceed button, got %s", body)
	}
	if !regexp.MustCompile(`>Next Round \([1-9][0-9]*\)</button>`).MatchString(body) {
		t.Fatalf("expected server-rendered review countdown label, got %s", body)
	}
	if strings.Contains(body, "data-review-") {
		t.Fatalf("judge review used client-side countdown attributes: %s", body)
	}
	if !strings.Contains(body, "room-player-winner") || !strings.Contains(body, "winner</span>") {
		t.Fatalf("expected sidebar winner highlight, got %s", body)
	}
	if !strings.Contains(body, "is-winning") {
		t.Fatalf("expected winning submission highlight, got %s", body)
	}

	body = renderRoom(guestSess)
	if !strings.Contains(body, `class="small text-muted"`) ||
		!regexp.MustCompile(`>Next round in [1-9][0-9]* seconds\.</span>`).MatchString(body) {
		t.Fatalf("expected non-judge JaWS countdown span, got %s", body)
	}
	if strings.Contains(body, "review-countdown-button") || strings.Contains(body, "data-review-") {
		t.Fatalf("non-judge review rendered a button or client-side countdown attributes: %s", body)
	}
}

func TestMissingRoomRendersMissingPanel(t *testing.T) {
	app, mux := testApp(t)
	handler := app.Middleware(mux)

	sess := newTestSession(t, app)

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
	sess := newTestSession(t, app)
	player := app.player(sess, req)
	app.setNickname(player, "Alice")
	room, err := app.createRoom(player)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	other := &game.Player{Nickname: "Bob", NicknameInput: "Bob"}
	otherRoom, err := app.Manager.CreateRoom(other, app.Catalog.DefaultDecks())
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

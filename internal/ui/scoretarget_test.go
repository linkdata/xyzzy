package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	if err := guestSlider.JawsSet(newScoreTargetElement(app, guestSlider), 8); err != nil {
		t.Fatalf("guestSlider.JawsSet() error = %v", err)
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
	if err := lockedSlider.JawsSet(newScoreTargetElement(app, lockedSlider), 10); err != nil {
		t.Fatalf("lockedSlider.JawsSet() error = %v", err)
	}
	if got := room.TargetScore(); got != 8 {
		t.Fatalf("TargetScore after in-game set = %d, want 8", got)
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

func newScoreTargetElement(app *App, slider bind.Binder[int]) *jaws.Element {
	return app.Jaws.NewRequest(nil).NewElement(jui.NewRange(bind.MakeSetterFloat64(slider)))
}

func newTestSession(t *testing.T, app *App, handler http.Handler) *jaws.Session {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	sess := app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected JaWS session")
	}
	return sess
}

func readUntilScoreTargetUpdate(ctx context.Context, conn *websocket.Conn, want string) error {
	for {
		_, body, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		text := string(body)
		if strings.Contains(text, `value="`+want+`"`) || strings.Contains(text, `>`+want+`<`) {
			return nil
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
					return nil
				}
			case what.Append, what.Replace:
				if strings.Contains(msg.Data, `value="`+want+`"`) || strings.Contains(msg.Data, `>`+want+`<`) {
					return nil
				}
			}
		}
	}
}

func testPlayableApp(t *testing.T) (*App, *http.ServeMux) {
	t.Helper()

	jw, err := jaws.New()
	if err != nil {
		t.Fatalf("jaws.New() error = %v", err)
	}
	t.Cleanup(jw.Close)
	go jw.Serve()

	catalog := testPlayableCatalog(t)
	app := New(jw, catalog, game.NewManagerWithOptions(catalog, game.Options{MinPlayers: 2}))
	mux := http.NewServeMux()
	if err := app.SetupRoutes(mux); err != nil {
		t.Fatalf("SetupRoutes() error = %v", err)
	}
	return app, mux
}

func testPlayableCatalog(t *testing.T) *deck.Catalog {
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

	catalog, err := deck.LoadFS(fsys)
	if err != nil {
		t.Fatalf("LoadFS() error = %v", err)
	}
	return catalog
}

package ui

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/jawstest"
	"github.com/linkdata/jaws/lib/jid"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/jaws/lib/what"
	"github.com/linkdata/jaws/lib/wire"
	"github.com/linkdata/xyzzy/internal/game"
)

func TestDeckSelectionRetainsControls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		app, _ := testPlayableApp(t)
		host, guest := &game.Player{Nickname: "Host"}, &game.Player{Nickname: "Guest"}
		room, err := app.Manager.CreateRoom(host, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = app.Manager.JoinRoom(room.Code(), guest); err != nil {
			t.Fatal(err)
		}
		type page struct {
			tr                 *jawstest.TestRequest
			records            []wire.WsMsg
			deck, count, start jid.Jid
			controls           []*jaws.Element
		}
		var pages []*page
		for _, player := range []*game.Player{host, host, guest} {
			p := &page{tr: jawstest.NewTestRequest(app.Jaws, nil)}
			<-p.tr.ReadyCh
			done := make(chan struct{})
			go func() {
				defer close(done)
				for msg := range p.tr.OutCh {
					p.records = append(p.records, msg)
				}
			}()
			t.Cleanup(func() { p.tr.Close(); <-p.tr.DoneCh; <-done })
			rw := jui.RequestWriter{Request: p.tr.Request, Writer: p.tr.Recorder}
			if err := rw.Template("section", "room_game_lobby.html", gameTemplateDot{templateDot: templateDot{App: app, Player: player}, Room: room}); err != nil {
				t.Fatal(err)
			}
			selectedDeck := app.Catalog.DefaultDecks()[0]
			p.deck = p.tr.GetElements(roomDeckTag{Room: room, Deck: selectedDeck})[0].Jid()
			for id := range immediateModeHTMLJIDs(p.tr.BodyString()) {
				e := p.tr.GetElementByJid(id)
				if e != nil {
					p.controls = append(p.controls, e)
				}
			}
			for _, e := range p.tr.GetElements(room.DeckSelectionTag()) {
				if _, ok := e.UI().(*jui.Span); ok {
					p.count = e.Jid()
				} else {
					p.start = e.Jid()
				}
			}
			pages = append(pages, p)
		}
		settle := func() { synctest.Wait(); time.Sleep(jaws.DefaultUpdateInterval + time.Millisecond); synctest.Wait() }
		settle()
		for _, p := range pages {
			if len(p.records) != 0 {
				t.Fatalf("initial commands: %+v", p.records)
			}
		}
		for step, value := range []string{"false", "false", "true"} {
			for _, p := range pages {
				p.records = nil
			}
			pages[0].tr.InCh <- wire.WsMsg{Jid: pages[0].deck, What: what.Input, Data: value}
			settle()
			unchanged := step == 1
			for i, p := range pages {
				if unchanged {
					if len(p.records) != 0 {
						t.Fatalf("unchanged input updated peer %d: %+v", i, p.records)
					}
					continue
				}
				expected := map[jid.Jid]what.What{p.count: what.Inner, p.start: what.SAttr}
				if value == "true" && i < 2 {
					expected[p.start] = what.RAttr
				}
				if i > 0 {
					expected[p.deck] = what.Value
				}
				if len(p.records) != len(expected) {
					t.Fatalf("peer %d records=%+v, expected targets=%v", i, p.records, expected)
				}
				for _, msg := range p.records {
					if want, ok := expected[msg.Jid]; !ok || msg.What != want {
						t.Fatalf("unexpected update %+v", msg)
					}
					if msg.Jid == p.count {
						wantCount := "0 black / 0 white selected"
						if value == "true" {
							wantCount = "50 black / 80 white selected"
						}
						if msg.Data != wantCount {
							t.Fatalf("count = %q, want %q", msg.Data, wantCount)
						}
					}
					if msg.Jid == p.deck && msg.Data != value {
						t.Fatalf("checkbox = %q, want %q", msg.Data, value)
					}
					delete(expected, msg.Jid)
				}
				for _, old := range p.controls {
					if p.tr.GetElementByJid(old.Jid()) != old {
						t.Fatal("deck edit replaced a control")
					}
				}
			}
		}
	})
}

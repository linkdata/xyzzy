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
			deckElements := p.tr.GetElements(roomDeckTag{Room: room, Deck: selectedDeck})
			if len(deckElements) != 1 {
				t.Fatalf("deck has %d Elements, want 1", len(deckElements))
			}
			p.deck = deckElements[0].Jid()
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
			p.tr.BcastCh <- wire.Message{What: what.Update}
			pages = append(pages, p)
		}
		settle := func() { synctest.Wait(); time.Sleep(jaws.DefaultUpdateInterval + time.Millisecond); synctest.Wait() }
		settle()
		for i, p := range pages {
			if i < 2 {
				if len(p.records) != 1 || p.records[0] != (wire.WsMsg{Jid: p.start, What: what.RAttr, Data: "disabled"}) {
					t.Fatalf("initial commands: %+v", p.records)
				}
			} else if len(p.records) != 0 {
				t.Fatalf("guest initial commands: %+v", p.records)
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
				type key struct {
					id   jid.Jid
					what what.What
				}
				expected := map[key]string{{p.count, what.Inner}: "0 black / 0 white selected"}
				if value == "true" {
					expected[key{p.count, what.Inner}] = "50 black / 80 white selected"
				}
				if i < 2 {
					expected[key{p.start, what.Inner}] = "Start Game"
					if value == "true" {
						expected[key{p.start, what.RAttr}] = "disabled"
					} else {
						expected[key{p.start, what.SAttr}] = "disabled\n"
					}
				}
				if i > 0 {
					expected[key{p.deck, what.Value}] = value
				}
				if len(p.records) != len(expected) {
					t.Fatalf("peer %d records=%+v, expected=%v", i, p.records, expected)
				}
				for _, msg := range p.records {
					k := key{msg.Jid, msg.What}
					if want, ok := expected[k]; !ok || msg.Data != want {
						t.Fatalf("unexpected update %+v", msg)
					}
					delete(expected, k)
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

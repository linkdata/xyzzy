package ui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/jawstest"
	"github.com/linkdata/jaws/lib/jid"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/jaws/lib/what"
	"github.com/linkdata/jaws/lib/wire"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

type selectionPage struct {
	tr      *jawstest.TestRequest
	records []wire.WsMsg
}

func newSelectionPage(t *testing.T, app *App, room *game.Room, player *game.Player, name string) *selectionPage {
	t.Helper()
	p := &selectionPage{tr: jawstest.NewTestRequest(app.Jaws, nil)}
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
	if err := rw.Template("section", name, gameTemplateDot{templateDot: templateDot{App: app, Player: player}, Room: room}); err != nil {
		t.Fatal(err)
	}
	return p
}

func settleSelection() {
	synctest.Wait()
	time.Sleep(jaws.DefaultUpdateInterval + time.Millisecond)
	synctest.Wait()
}

func (p *selectionPage) element(t *testing.T, tag any) *jaws.Element {
	t.Helper()
	elements := p.tr.GetElements(tag)
	if len(elements) != 1 {
		t.Fatalf("tag %#v has %d elements, want 1", tag, len(elements))
	}
	return elements[0]
}

func (p *selectionPage) click(e *jaws.Element) {
	p.tr.InCh <- wire.WsMsg{Jid: e.Jid(), What: what.Click, Data: (jaws.Click{}).String()}
}

func TestHandSelectionUpdatesOnlyAffectedCards(t *testing.T) {
	for _, pick := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(pick), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				app, _ := testPlayableApp(t)
				// Configure the immutable catalog before publishing the game or any UI.
				for _, card := range app.Catalog.BlackCards {
					card.Pick = pick
				}
				host, player, other := &game.Player{Nickname: "Host"}, &game.Player{Nickname: "Player"}, &game.Player{Nickname: "Other"}
				room, err := app.Manager.CreateRoom(host, nil)
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range []*game.Player{player, other} {
					if _, err = app.Manager.JoinRoom(room.Code(), p); err != nil {
						t.Fatal(err)
					}
				}
				if err = room.Start(host); err != nil {
					t.Fatal(err)
				}
				pages := []*selectionPage{
					newSelectionPage(t, app, room, player, "room_game_playing.html"),
					newSelectionPage(t, app, room, player, "room_game_playing.html"),
					newSelectionPage(t, app, room, other, "room_game_playing.html"),
				}
				hand := room.HandFor(player)
				type step struct {
					card      int
					orders    map[int]int
					readiness int
				}
				steps := []step{{0, map[int]int{0: 1}, 1}, {1, map[int]int{0: 0, 1: 1}, 0}, {1, map[int]int{1: 0}, -1}}
				if pick == 2 {
					steps = []step{{0, map[int]int{0: 1}, 0}, {1, map[int]int{1: 2}, 1}, {2, nil, 0}, {0, map[int]int{0: 0, 1: 1}, -1}, {1, map[int]int{1: 0}, 0}}
				}
				if pick == 3 {
					steps = []step{{0, map[int]int{0: 1}, 0}, {1, map[int]int{1: 2}, 0}, {2, map[int]int{2: 3}, 1}, {3, nil, 0}, {0, map[int]int{0: 0, 1: 1, 2: 2}, -1}, {1, map[int]int{1: 0, 2: 1}, 0}}
				}
				settleSelection()
				for _, p := range pages {
					if len(p.records) != 0 {
						t.Fatalf("initial commands: %+v", p.records)
					}
				}
				for _, s := range steps {
					retained := make([]map[jid.Jid]*jaws.Element, len(pages))
					for i, p := range pages {
						p.records = nil
						retained[i] = make(map[jid.Jid]*jaws.Element)
						for _, card := range hand {
							for _, e := range p.tr.GetElements(room.HandCardTag(player, card)) {
								retained[i][e.Jid()] = e
							}
						}
					}
					pages[0].click(pages[0].element(t, room.HandCardTag(player, hand[s.card])))
					settleSelection()
					for i, p := range pages {
						if i == 2 {
							if len(p.records) != 0 {
								t.Fatalf("other player received %+v", p.records)
							}
							continue
						}
						expected := make(map[jid.Jid]int)
						for index, order := range s.orders {
							expected[p.element(t, room.HandCardTag(player, hand[index])).Jid()] = order
						}
						action := p.element(t, player.HandReadinessTag()).Jid()
						count := 2 * len(expected)
						if s.readiness != 0 {
							count++
						}
						if len(p.records) != count {
							t.Fatalf("pick %d step %+v: updates=%+v, want %d", pick, s, p.records, count)
						}
						seen := make(map[struct {
							id   jid.Jid
							what what.What
						}]bool)
						bytes := 0
						for _, msg := range p.records {
							bytes += len(msg.Append(nil))
							key := struct {
								id   jid.Jid
								what what.What
							}{msg.Jid, msg.What}
							if seen[key] {
								t.Fatalf("duplicate update: %+v", msg)
							}
							seen[key] = true
							if order, ok := expected[msg.Jid]; ok {
								switch msg.What {
								case what.Inner:
									if order == 0 && strings.Contains(msg.Data, "card-selection-order") {
										t.Fatalf("stale ordinal: %s", msg.Data)
									}
									if order > 0 && !strings.Contains(msg.Data, fmt.Sprintf("#%d", order)) {
										t.Fatalf("missing ordinal: %s", msg.Data)
									}
								case what.SAttr:
									if msg.Data != "aria-pressed\n"+strconv.FormatBool(order > 0) {
										t.Fatalf("pressed state: %+v", msg)
									}
								default:
									t.Fatalf("unexpected card command: %+v", msg)
								}
							} else if msg.Jid == action && s.readiness != 0 {
								wantWhat, wantData := what.SAttr, "disabled\n"
								if s.readiness > 0 {
									wantWhat, wantData = what.RAttr, "disabled"
								}
								if msg.What != wantWhat || msg.Data != wantData {
									t.Fatalf("readiness: %+v", msg)
								}
							} else {
								t.Fatalf("unrelated update: %+v", msg)
							}
						}
						if bytes > 300*len(expected)+40 {
							t.Fatalf("selection sent %d bytes", bytes)
						}
						for id, e := range retained[i] {
							if p.tr.GetElementByJid(id) != e {
								t.Fatal("selection replaced a card")
							}
						}
						t.Logf("pick=%d affected=%d commands=%d bytes=%d", pick, len(expected), len(p.records), bytes)
					}
				}
			})
		})
	}
}

func TestJudgingSelectionUpdatesOnlyAttributes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		app, _ := testPlayableApp(t)
		host, a, b := &game.Player{Nickname: "Host"}, &game.Player{Nickname: "A"}, &game.Player{Nickname: "B"}
		room, err := app.Manager.CreateRoom(host, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range []*game.Player{a, b} {
			if _, err = app.Manager.JoinRoom(room.Code(), p); err != nil {
				t.Fatal(err)
			}
		}
		if err = room.Start(host); err != nil {
			t.Fatal(err)
		}
		for _, p := range []*game.Player{a, b} {
			if err = room.PlayCards(p, []*deck.WhiteCard{room.HandFor(p)[0]}); err != nil {
				t.Fatal(err)
			}
		}
		pages := []*selectionPage{newSelectionPage(t, app, room, host, "room_game_judging.html"), newSelectionPage(t, app, room, host, "room_game_judging.html"), newSelectionPage(t, app, room, a, "room_game_judging.html")}
		submissions := room.Submissions()
		settleSelection()
		for _, p := range pages {
			if len(p.records) != 0 {
				t.Fatalf("initial commands: %+v", p.records)
			}
		}
		for step, index := range []int{0, 1, 1} {
			for _, p := range pages {
				p.records = nil
			}
			clicked := pages[0].element(t, room.SubmissionTag(host, submissions[index]))
			pages[0].click(clicked)
			settleSelection()
			for i, p := range pages {
				if i == 2 {
					if len(p.records) != 0 {
						t.Fatalf("non-judge updated: %+v", p.records)
					}
					continue
				}
				expected := map[jid.Jid]wire.WsMsg{}
				add := func(index int, selected bool) {
					e := p.element(t, room.SubmissionTag(host, submissions[index]))
					expected[e.Jid()] = wire.WsMsg{Jid: e.Jid(), What: what.SAttr, Data: "aria-pressed\n" + strconv.FormatBool(selected)}
				}
				switch step {
				case 0:
					add(0, true)
				case 1:
					add(0, false)
					add(1, true)
				case 2:
					add(1, false)
				}
				if step != 1 {
					e := p.element(t, host.JudgeReadinessTag())
					msg := wire.WsMsg{Jid: e.Jid(), What: what.SAttr, Data: "disabled\n"}
					if step == 0 {
						msg.What = what.RAttr
						msg.Data = "disabled"
					}
					expected[e.Jid()] = msg
				}
				if len(p.records) != len(expected) {
					t.Fatalf("updates=%+v, want %+v", p.records, expected)
				}
				for _, msg := range p.records {
					if want, ok := expected[msg.Jid]; !ok || msg != want {
						t.Fatalf("unexpected update: %+v", msg)
					}
					delete(expected, msg.Jid)
				}
			}
			if pages[0].tr.GetElementByJid(clicked.Jid()) != clicked {
				t.Fatal("judge selection replaced a card")
			}
		}
		for _, p := range pages {
			p.records = nil
		}
		pages[2].click(pages[2].element(t, room.SubmissionTag(a, submissions[0])))
		settleSelection()
		for _, p := range pages {
			if len(p.records) != 0 {
				t.Fatalf("non-judge click updated: %+v", p.records)
			}
		}
	})
}

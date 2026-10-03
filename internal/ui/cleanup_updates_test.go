package ui

import (
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/jawstest"
	"github.com/linkdata/jaws/lib/bind"
	"github.com/linkdata/jaws/lib/jid"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/jaws/lib/what"
	"github.com/linkdata/jaws/lib/wire"
	"github.com/linkdata/xyzzy/internal/game"
)

func TestCleanupPublishesEveryAffectedRoom(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		jw, err := jaws.New()
		if err != nil {
			t.Fatal(err)
		}
		defer jw.Close()
		go jw.Serve()
		catalog := testCatalog(t)
		app := New(jw, catalog, game.NewManager(catalog))
		sess := newTestSession(t, app)
		tr := jawstest.NewTestRequest(jw, nil)
		<-tr.ReadyCh
		var records []wire.WsMsg
		drained := make(chan struct{})
		go func() {
			defer close(drained)
			for msg := range tr.OutCh {
				records = append(records, msg)
			}
		}()
		defer func() { tr.Close(); <-tr.DoneCh; <-drained }()
		rw := jui.RequestWriter{Request: tr.Request, Writer: tr.Recorder}
		expected := make(map[jid.Jid]string)
		const roomCount = 110
		for range roomCount {
			room, err := app.Manager.CreateRoom(&game.Player{Nickname: "Expired"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = app.Manager.JoinRoom(room.Code(), &game.Player{Session: sess, Nickname: "Present"}); err != nil {
				t.Fatal(err)
			}
			if err = rw.Span(bind.StringGetterFunc(func(*jaws.Element) string {
				return strconv.Itoa(room.PlayerCount())
			}, room)); err != nil {
				t.Fatal(err)
			}
			expected[tr.GetElements(room)[0].Jid()] = "1"
		}
		if err = rw.Span(bind.StringGetterFunc(func(*jaws.Element) string {
			return strconv.Itoa(len(app.Manager.Rooms()))
		}, app.Manager)); err != nil {
			t.Fatal(err)
		}
		expected[tr.GetElements(app.Manager)[0].Jid()] = strconv.Itoa(roomCount)
		synctest.Wait()
		app.cleanupExpired()
		synctest.Wait()
		time.Sleep(jaws.DefaultUpdateInterval + time.Millisecond)
		synctest.Wait()
		if len(records) != len(expected) {
			t.Fatalf("updates = %d, want %d", len(records), len(expected))
		}
		for _, msg := range records {
			want, ok := expected[msg.Jid]
			if !ok || msg.What != what.Inner || msg.Data != want {
				t.Fatalf("unexpected update: %+v", msg)
			}
			delete(expected, msg.Jid)
		}
	})
}

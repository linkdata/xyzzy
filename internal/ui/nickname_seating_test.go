package ui

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/lib/what"
	"github.com/linkdata/jaws/lib/wire"
)

func TestRoomConnectReconcilesNicknameAcrossPages(t *testing.T) {
	h := newLiveHarness(t)
	_, host := livePlayer(t, h, "Alice")
	room, err := h.app.createRoom(host)
	if err != nil {
		t.Fatal(err)
	}
	client := h.newClient(t)
	// Restore the duplicate name without leaving a pending SetNickname update.
	client.Jar.SetCookies(h.base, []*http.Cookie{{
		Name: h.app.nicknameCookieName(), Value: base64.RawURLEncoding.EncodeToString([]byte("Alice")), Path: "/",
	}})
	siblingHTML := h.getWithClient(t, client, "/")
	sess := h.sessionForClient(t, client)
	player := h.app.player(sess, nil)
	siblingRequest := immediateModeRequestForHTML(t, sess, siblingHTML)
	siblingConn, siblingCancel := h.connectWithClient(t, client, siblingHTML)
	defer siblingCancel()
	siblingReader := newImmediateModeWireReader(t, siblingConn)
	syncImmediateModeRequest(t, siblingConn, siblingRequest, siblingReader, "before-join")

	roomHTML := h.getWithClient(t, client, h.app.RoomURL(room.Code()))
	roomRequest := immediateModeRequestForHTML(t, sess, roomHTML)
	if player.Room() != nil || player.NicknameInputValue() != "Alice" {
		t.Fatal("room GET changed membership or nickname before connecting")
	}
	inputs := make([]*jaws.Element, 0, 2)
	for _, rq := range []*jaws.Request{siblingRequest, roomRequest} {
		elements := rq.GetElements(player.NicknameField().JawsGetTag())
		if len(elements) != 1 {
			t.Fatalf("nickname field has %d Elements, want 1", len(elements))
		}
		inputs = append(inputs, elements[0])
	}
	roomConn, roomCancel := h.connectWithClient(t, client, roomHTML)
	defer roomCancel()
	roomReader := newImmediateModeWireReader(t, roomConn)
	for i, reader := range []immediateModeWireReader{siblingReader, roomReader} {
		ctx, cancel := context.WithTimeout(t.Context(), immediateModeTestTimeout)
		err = reader.readUntil(ctx, func(msg wire.WsMsg) bool {
			return msg.Jid == inputs[i].Jid() && msg.What == what.Value && msg.Data == "Alice-2"
		})
		cancel()
		if err != nil {
			t.Fatalf("page %d did not reconcile nickname input: %v", i, err)
		}
	}
	if player.Room() != room || player.NicknameValue() != "Alice-2" || player.NicknameInputValue() != "Alice-2" {
		t.Fatal("connection did not seat the player with a unique nickname")
	}
	for i, rq := range []*jaws.Request{siblingRequest, roomRequest} {
		if rq.GetElementByJid(inputs[i].Jid()) != inputs[i] {
			t.Fatalf("page %d replaced its nickname input", i)
		}
	}
}

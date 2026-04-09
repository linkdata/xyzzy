package ui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/game"
)

var jawsKeyRe = regexp.MustCompile(`<meta name="jawsKey" content="([^"]+)"`)

type liveHarness struct {
	app    *App
	server *httptest.Server
	client *http.Client
	base   *url.URL
}

func newLiveHarness(t *testing.T) *liveHarness {
	t.Helper()

	jw, err := jaws.New()
	if err != nil {
		t.Fatalf("jaws.New() error = %v", err)
	}
	catalog := testCatalog(t)
	app := New(jw, catalog, game.NewManager(catalog, nil))
	mux := http.NewServeMux()
	if err := app.SetupRoutes(mux); err != nil {
		t.Fatalf("SetupRoutes() error = %v", err)
	}

	go jw.Serve()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	t.Cleanup(jw.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	client := &http.Client{Jar: jar}

	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	return &liveHarness{
		app:    app,
		server: server,
		client: client,
		base:   base,
	}
}

func (h *liveHarness) newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	return &http.Client{Jar: jar}
}

func (h *liveHarness) get(t *testing.T, path string) string {
	return h.getWithClient(t, h.client, path)
}

func (h *liveHarness) getWithClient(t *testing.T, client *http.Client, path string) string {
	t.Helper()
	resp, err := client.Get(h.server.URL + path)
	if err != nil {
		t.Fatalf("GET %s error = %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll(%s) error = %v", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d body=%s", path, resp.StatusCode, body)
	}
	return string(body)
}

func (h *liveHarness) cookies() []*http.Cookie {
	return h.cookiesFor(h.client)
}

func (h *liveHarness) cookiesFor(client *http.Client) []*http.Cookie {
	return client.Jar.Cookies(h.base)
}

func (h *liveHarness) session(t *testing.T) *jaws.Session {
	return h.sessionForClient(t, h.client)
}

func (h *liveHarness) sessionForClient(t *testing.T, client *http.Client) *jaws.Session {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, h.server.URL+"/", nil)
	req.Host = h.base.Host
	req.URL.Host = h.base.Host
	req.URL.Scheme = h.base.Scheme
	req.RemoteAddr = "127.0.0.1:12345"
	for _, cookie := range h.cookiesFor(client) {
		req.AddCookie(cookie)
	}
	sess := h.app.Jaws.GetSession(req)
	if sess == nil {
		t.Fatal("expected JaWS session")
	}
	return sess
}

func (h *liveHarness) connect(t *testing.T, html string) (*websocket.Conn, context.CancelFunc) {
	return h.connectWithCookies(t, html, h.cookies())
}

func (h *liveHarness) connectWithCookies(t *testing.T, html string, cookies []*http.Cookie) (*websocket.Conn, context.CancelFunc) {
	t.Helper()
	match := jawsKeyRe.FindStringSubmatch(html)
	if len(match) != 2 {
		t.Fatalf("did not find jawsKey in html: %s", html)
	}
	wsURL := strings.Replace(h.server.URL, "http://", "ws://", 1) + "/jaws/" + url.PathEscape(match[1])
	hdr := http.Header{}
	hdr.Set("Origin", h.server.URL)
	hdr.Set("Cookie", cookieHeader(cookies))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		cancel()
		t.Fatalf("websocket.Dial() error = %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
		cancel()
	})
	return conn, cancel
}

func cookieHeader(cookies []*http.Cookie) string {
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
}

func readUntilContains(ctx context.Context, conn *websocket.Conn, needle string) (string, error) {
	for {
		_, body, err := conn.Read(ctx)
		if err != nil {
			return "", err
		}
		text := string(body)
		if strings.Contains(text, needle) {
			return text, nil
		}
	}
}

func TestLobbyPageReceivesLiveRoomUpdates(t *testing.T) {
	h := newLiveHarness(t)

	html := h.get(t, "/")
	conn, cancel := h.connect(t, html)
	defer cancel()

	otherClient := h.newClient(t)
	h.getWithClient(t, otherClient, "/")
	otherSession := h.sessionForClient(t, otherClient)
	h.app.setNickname(otherSession, "Bob")
	room, err := h.app.createRoom(otherSession)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	msg, err := readUntilContains(ctx, conn, room.Code())
	if err != nil {
		t.Fatalf("readUntilContains() error = %v", err)
	}
	if !strings.Contains(msg, "Bob") {
		t.Fatalf("expected lobby update to mention host name, got %s", msg)
	}
}

func TestRoomPageReceivesLiveRoomUpdates(t *testing.T) {
	h := newLiveHarness(t)

	h.get(t, "/")
	sess := h.session(t)
	h.app.setNickname(sess, "Alice")
	room, err := h.app.createRoom(sess)
	if err != nil {
		t.Fatalf("createRoom() error = %v", err)
	}

	html := h.get(t, "/room/"+room.Code())
	conn, cancel := h.connect(t, html)
	defer cancel()

	if err := room.ToggleDeck(h.app.playerID(sess), "extra", true); err != nil {
		t.Fatalf("ToggleDeck() error = %v", err)
	}
	h.app.DirtyRoom(room)

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	want := fmt.Sprintf("%d unique black cards and %d unique white cards selected", 2, 4)
	msg, err := readUntilContains(ctx, conn, want)
	if err != nil {
		t.Fatalf("readUntilContains() error = %v", err)
	}
	if !strings.Contains(msg, "Deck Selection") {
		t.Fatalf("expected room update to include deck panel markup, got %s", msg)
	}
}

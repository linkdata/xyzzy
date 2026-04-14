package ui

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/linkdata/jaws"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

func testCatalog(t *testing.T) *deck.Catalog {
	t.Helper()
	fsys := fstest.MapFS{
		"assets/cards/black/b1.json":    {Data: []byte(`{"id":"b1","text":"Question?"}`)},
		"assets/cards/black/b2.json":    {Data: []byte(`{"id":"b2","text":"Another question?"}`)},
		"assets/cards/white/w1.json":    {Data: []byte(`{"id":"w1","text":"Answer 1"}`)},
		"assets/cards/white/w2.json":    {Data: []byte(`{"id":"w2","text":"Answer 2"}`)},
		"assets/cards/white/w3.json":    {Data: []byte(`{"id":"w3","text":"Answer 3"}`)},
		"assets/cards/white/w4.json":    {Data: []byte(`{"id":"w4","text":"Answer 4"}`)},
		"assets/decks/base/deck.json":   {Data: []byte(`{"id":"base","name":"Base","enabled_by_default":true}`)},
		"assets/decks/base/black.json":  {Data: []byte(`["b1"]`)},
		"assets/decks/base/white.json":  {Data: []byte(`["w1","w2","w3"]`)},
		"assets/decks/extra/deck.json":  {Data: []byte(`{"id":"extra","name":"Extra"}`)},
		"assets/decks/extra/black.json": {Data: []byte(`["b2"]`)},
		"assets/decks/extra/white.json": {Data: []byte(`["w4"]`)},
	}
	catalog, err := deck.LoadFS(fsys)
	if err != nil {
		t.Fatalf("LoadFS() error = %v", err)
	}
	return catalog
}

func testApp(t *testing.T) (*App, *http.ServeMux) {
	t.Helper()
	jw, err := jaws.New()
	if err != nil {
		t.Fatalf("jaws.New() error = %v", err)
	}
	t.Cleanup(jw.Close)
	go jw.Serve()
	catalog := testCatalog(t)
	app := New(jw, catalog, game.NewManager(catalog))
	mux := http.NewServeMux()
	if err := app.SetupRoutes(mux); err != nil {
		t.Fatalf("SetupRoutes() error = %v", err)
	}
	return app, mux
}

var jawsKeyRe = regexp.MustCompile(`<meta name="jawsKey" content="([^"]+)"`)

type liveHarness struct {
	app    *App
	server *httptest.Server
	client *http.Client
	base   *url.URL
}

func newLiveHarness(t *testing.T) *liveHarness {
	t.Helper()
	return newHarnessWithCatalog(t, testCatalog(t), game.Options{})
}

func newPlayableLiveHarness(t *testing.T) *liveHarness {
	t.Helper()
	return newHarnessWithCatalog(t, testPlayableCatalog(t), game.Options{})
}

func newHarnessWithCatalog(t *testing.T, catalog *deck.Catalog, opts game.Options) *liveHarness {
	t.Helper()
	jw, err := jaws.New()
	if err != nil {
		t.Fatalf("jaws.New() error = %v", err)
	}
	app := New(jw, catalog, game.NewManagerWithOptions(catalog, opts))
	mux := http.NewServeMux()
	if err := app.SetupRoutes(mux); err != nil {
		t.Fatalf("SetupRoutes() error = %v", err)
	}

	go jw.Serve()
	server := httptest.NewServer(app.Middleware(mux))
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

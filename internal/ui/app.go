package ui

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"path"
	"strings"

	"github.com/linkdata/jaws"
	"github.com/linkdata/jaws/jawsboot"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/staticserve"
	"github.com/linkdata/xyzzy"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

const (
	sessionKeyPlayer  = "player"
	nicknameCookieTTL = 365 * 24 * 60 * 60
)

// App serves the xyzzy HTTP and JaWS user interface.
type App struct {
	Jaws              *jaws.Jaws
	Catalog           *deck.Catalog
	Manager           *game.Manager
	csrfSecret        [32]byte
	createRoomLimiter *createRoomLimiter
}

// New returns an App using the supplied JaWS server, catalog, and manager.
//
// It installs the JaWS dirty notifier on manager. New panics if the operating
// system cannot provide cryptographic randomness for CSRF protection.
func New(jw *jaws.Jaws, catalog *deck.Catalog, manager *game.Manager) *App {
	if manager != nil && jw != nil {
		manager.SetDirty(jw.Dirty)
	}
	result := &App{
		Jaws:              jw,
		Catalog:           catalog,
		Manager:           manager,
		createRoomLimiter: newCreateRoomLimiter(),
	}
	if _, err := rand.Read(result.csrfSecret[:]); err != nil {
		panic(err)
	}
	return result
}

// SetupRoutes registers the App's HTTP routes and templates on mux.
func (a *App) SetupRoutes(mux *http.ServeMux) (err error) {
	var templates *template.Template
	if templates, err = template.New("root").ParseFS(xyzzy.Assets, "assets/templates/*.html"); err == nil {
		if err = a.Jaws.AddTemplateLookuper(templates); err == nil {
			if err = a.Jaws.Setup(
				mux.Handle, "/static",
				jawsboot.Setup,
				staticserve.MustNewFS(xyzzy.Assets, "assets/static", "images/favicon.svg", "app.css"),
			); err == nil {
				mux.Handle("GET /jaws/", a.Jaws)
				mux.Handle("GET /robots.txt", http.HandlerFunc(a.serveRobots))
				mux.Handle("GET /.well-known/security.txt", http.HandlerFunc(a.serveSecurityTxt))
				mux.Handle("GET /{$}", a.Jaws.SessionMiddleware(http.HandlerFunc(a.serveLobby)))
				mux.Handle("GET /create-room", http.HandlerFunc(a.serveCreateRoom))
				mux.Handle("POST /create-room", http.HandlerFunc(a.serveCreateRoom))
				mux.Handle("GET /room/{code}", a.Jaws.SessionMiddleware(http.HandlerFunc(a.serveRoom)))
				mux.Handle("GET /{path...}", http.HandlerFunc(a.serveNotFound))
			}
		}
	}
	return
}

// Middleware applies the App's HTTP security headers to next.
func (a *App) Middleware(next http.Handler) http.Handler {
	return a.Jaws.SecureHeadersMiddleware(next)
}

func (a *App) serveLobby(w http.ResponseWriter, r *http.Request) {
	sess := a.Jaws.GetSession(r)
	if sess == nil {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	player := a.player(sess, r)
	a.cleanupExpired()
	if player.Room() != nil {
		a.leaveRoom(player)
	}
	a.syncNicknameCookie(w, r, player)
	jui.Handler(a.Jaws, "index.html", templateDot{App: a, Player: player}).ServeHTTP(w, r)
}

func (a *App) serveRoom(w http.ResponseWriter, r *http.Request) {
	sess := a.Jaws.GetSession(r)
	if sess == nil {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	player := a.player(sess, r)
	a.cleanupExpired()
	roomCode := normalizeRoomCode(r.PathValue("code"))
	if player.Room() == nil {
		_, _ = a.joinRoom(player, roomCode)
	}
	if current := player.Room(); current != nil && current.Code() != roomCode {
		http.Redirect(w, r, a.RoomURL(current.Code()), http.StatusSeeOther)
		return
	}
	a.syncNicknameCookie(w, r, player)
	jui.Handler(a.Jaws, "room.html", templateDot{App: a, Player: player}).ServeHTTP(w, r)
}

func (a *App) serveCreateRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !a.validRequestOrigin(r) || !a.validCSRF(r) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	if retryAfter, ok := a.createRoomLimiter.Allow(clientIP(r)); !ok {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
		return
	}
	sess := a.Jaws.GetSession(r)
	if sess == nil {
		http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)
		return
	}
	player := a.player(sess, r)
	a.cleanupExpired()
	if current := player.Room(); current != nil {
		http.Redirect(w, r, a.RoomURL(current.Code()), http.StatusSeeOther)
		return
	}
	room, err := a.createRoom(player)
	if err != nil {
		http.Error(w, a.Jaws.Log(err).Error(), http.StatusInternalServerError)
		return
	}
	a.syncNicknameCookie(w, r, player)
	http.Redirect(w, r, a.RoomURL(room.Code()), http.StatusSeeOther)
}

func (a *App) player(sess *jaws.Session, r *http.Request) (result *game.Player) {
	if result, _ = sess.Get(sessionKeyPlayer).(*game.Player); result != nil {
		if result.Session == nil {
			result.Session = sess
		}
		if result.Room() == nil {
			if nickname := result.NicknameValue(); nickname == "" {
				a.Manager.SetNickname(result, generateNickname())
			} else if result.NicknameInputValue() == "" {
				a.Manager.SetNickname(result, nickname)
			}
		}
		return
	}
	nickname := a.nicknameFromCookie(r)
	if nickname == "" {
		nickname = generateNickname()
	} else {
		nickname = game.NormalizeNickname(nickname)
	}
	result = &game.Player{
		Session:       sess,
		Nickname:      nickname,
		NicknameInput: nickname,
	}
	sess.Set(sessionKeyPlayer, result)
	return
}

func (a *App) cleanupExpired() {
	affected := a.Manager.CleanupExpiredSessions()
	if len(affected) > 0 {
		tags := []any{a.Manager}
		for _, room := range affected {
			tags = append(tags, room)
		}
		a.Jaws.Dirty(tags...)
	}
}

func (a *App) nicknameCookieName() (result string) {
	result = a.sessionCookieName() + "_nickname"
	return
}

func (a *App) nicknameFromCookie(r *http.Request) (result string) {
	if r != nil {
		cookie, err := r.Cookie(a.nicknameCookieName())
		if err == nil && cookie.Value != "" {
			if raw, err := base64.RawURLEncoding.DecodeString(cookie.Value); err == nil {
				result = strings.TrimSpace(string(raw))
			}
		}
	}
	return
}

func (a *App) setNicknameCookie(w http.ResponseWriter, r *http.Request, nickname string) {
	nickname = strings.TrimSpace(nickname)
	value := ""
	if nickname != "" {
		value = base64.RawURLEncoding.EncodeToString([]byte(nickname))
	}
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure follows the request scheme; HttpOnly and SameSite are set below.
		Name:     a.nicknameCookieName(),
		Value:    value,
		Path:     "/",
		MaxAge:   nicknameCookieTTL,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(r),
	})
}

func (a *App) syncNicknameCookie(w http.ResponseWriter, r *http.Request, player *game.Player) {
	if player != nil {
		nickname := strings.TrimSpace(a.playerNickname(player))
		if nickname == "" {
			a.Manager.SetNickname(player, generateNickname())
			nickname = a.playerNickname(player)
		}
		if nickname != a.nicknameFromCookie(r) {
			a.setNicknameCookie(w, r, nickname)
		}
	}
}

func (a *App) playerNickname(player *game.Player) (result string) {
	if player != nil {
		if room := player.Room(); room != nil {
			result = room.NicknameFor(player)
			return
		}
		result = player.NicknameValue()
	}
	return
}

func generateNickname() (result string) {
	var b [3]byte
	_, _ = rand.Read(b[:])
	result = fmt.Sprintf("Player%X", b)
	return
}

func (a *App) createRoom(player *game.Player) (room *game.Room, err error) {
	if room, err = a.Manager.CreateRoom(player, a.Catalog.DefaultDecks()); err == nil {
		a.Jaws.Dirty(a.Manager, room, player)
	}
	return
}

func (a *App) joinRoom(player *game.Player, roomCode string) (room *game.Room, err error) {
	if room, err = a.Manager.JoinRoom(roomCode, player); err == nil {
		a.Jaws.Dirty(a.Manager, room, player)
	}
	return
}

func (a *App) leaveRoom(player *game.Player) (room *game.Room) {
	room, _ = a.Manager.LeaveRoom(player)
	a.Jaws.Dirty(a.Manager, room, player)
	return
}

// RoomURL returns the canonical path for code, or the lobby path for an empty code.
func (a *App) RoomURL(code string) (result string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		result = "/"
		return
	}
	result = path.Join("/room", code)
	return
}

func requestIsSecure(r *http.Request) (result bool) {
	if r != nil {
		result = r.TLS != nil
		result = result || strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]) == "https"
	}
	return
}

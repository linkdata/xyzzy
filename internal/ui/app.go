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

type App struct {
	Jaws    *jaws.Jaws
	Catalog *deck.Catalog
	Manager *game.Manager
}

func New(jw *jaws.Jaws, catalog *deck.Catalog, manager *game.Manager) (result *App) {
	if manager != nil && jw != nil {
		manager.SetDirty(jw.Dirty)
	}
	result = &App{Jaws: jw, Catalog: catalog, Manager: manager}
	return
}

func (a *App) SetupRoutes(mux *http.ServeMux) (errResult error) {
	templates, err := template.New("root").ParseFS(xyzzy.Assets, "assets/templates/*.html")
	if err != nil {
		errResult = err
		return
	}
	if err := a.Jaws.AddTemplateLookuper(templates); err != nil {
		errResult = err
		return
	}
	if err := a.Jaws.Setup(mux.Handle, "/static",
		jawsboot.Setup,
		staticserve.MustNewFS(xyzzy.Assets, "assets/static", "images/favicon.svg", "app.css", "app.js"),
	); err != nil {
		errResult = err
		return
	}
	mux.Handle("GET /jaws/", a.Jaws)
	mux.Handle("GET /", http.HandlerFunc(a.serveLobby))
	mux.Handle("GET /create-room", http.HandlerFunc(a.serveCreateRoom))
	mux.Handle("GET /room/{code}", http.HandlerFunc(a.serveRoom))
	errResult = nil
	return
}

func (a *App) Middleware(next http.Handler) (result http.Handler) {
	result = a.Jaws.Session(a.Jaws.SecureHeadersMiddleware(next))
	return
}

func (a *App) serveLobby(w http.ResponseWriter, r *http.Request) {
	sess := a.session(r)
	player := a.player(sess, r)
	a.cleanupExpired()
	if player.Room != nil {
		a.leaveRoom(player)
	}
	a.syncNicknameCookie(w, r, player)
	if err := a.renderTemplate(w, r, "index.html", a.makeTemplateDot(player)); err != nil {
		a.Jaws.Log(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *App) serveRoom(w http.ResponseWriter, r *http.Request) {
	sess := a.session(r)
	player := a.player(sess, r)
	a.cleanupExpired()
	roomCode := strings.ToUpper(strings.TrimSpace(r.PathValue("code")))
	if current := player.Room; current != nil && current.Code() != roomCode {
		http.Redirect(w, r, "/room/"+current.Code(), http.StatusSeeOther)
		return
	}
	if player.Room == nil {
		if room := a.Manager.Room(roomCode); room != nil && room.CanJoin(player) {
			_, _ = a.joinRoom(player, roomCode)
		}
	}
	a.syncNicknameCookie(w, r, player)
	if err := a.renderTemplate(w, r, "room.html", a.makeTemplateDot(player)); err != nil {
		a.Jaws.Log(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *App) serveCreateRoom(w http.ResponseWriter, r *http.Request) {
	sess := a.session(r)
	player := a.player(sess, r)
	a.cleanupExpired()
	if player.Room != nil {
		http.Redirect(w, r, a.roomURL(player.Room.Code()), http.StatusSeeOther)
		return
	}
	room, err := a.createRoom(player)
	if err != nil {
		a.Jaws.Log(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.syncNicknameCookie(w, r, player)
	http.Redirect(w, r, a.roomURL(room.Code()), http.StatusSeeOther)
}

func (a *App) renderTemplate(w http.ResponseWriter, r *http.Request, name string, dot any) (errResult error) {
	req := a.Jaws.NewRequest(r)
	errResult = req.NewElement(jui.Template{Name: name, Dot: dot}).JawsRender(w, nil)
	return
}

func (a *App) makeTemplateDot(player *game.Player) (result templateDot) {
	result = templateDot{App: a, Player: player, Room: player.Room}
	return
}

func (a *App) session(r *http.Request) (result *jaws.Session) {
	result = a.Jaws.GetSession(r)
	if result == nil {
		panic("ui.App handlers require JaWS session middleware")
	}
	return
}

func (a *App) player(sess *jaws.Session, r *http.Request) (result *game.Player) {
	if result, _ = sess.Get(sessionKeyPlayer).(*game.Player); result != nil {
		if result.Session == nil {
			result.Session = sess
		}
		if result.Nickname == "" {
			result.Nickname = generateNickname()
		}
		if result.NicknameInput == "" {
			result.NicknameInput = result.Nickname
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
	if len(affected) == 0 {
		return
	}
	tags := []any{a.Manager}
	for _, room := range affected {
		tags = append(tags, room)
	}
	a.Jaws.Dirty(tags...)
}

func (a *App) nicknameCookieName() (result string) {
	name := strings.TrimSpace(a.Jaws.CookieName)
	if name == "" {
		name = "jaws"
	}
	result = name + "_nickname"
	return
}

func (a *App) nicknameFromCookie(r *http.Request) (result string) {
	if r == nil {
		result = ""
		return
	}
	cookie, err := r.Cookie(a.nicknameCookieName())
	if err != nil || cookie.Value == "" {
		result = ""
		return
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		result = ""
		return
	}
	result = strings.TrimSpace(string(raw))
	return
}

func (a *App) setNicknameCookie(w http.ResponseWriter, r *http.Request, nickname string) {
	nickname = strings.TrimSpace(nickname)
	value := ""
	if nickname != "" {
		value = base64.RawURLEncoding.EncodeToString([]byte(nickname))
	}
	http.SetCookie(w, &http.Cookie{
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
	if player == nil {
		return
	}
	nickname := strings.TrimSpace(player.Nickname)
	if nickname == "" {
		nickname = generateNickname()
		player.Nickname = nickname
		if player.NicknameInput == "" {
			player.NicknameInput = nickname
		}
	}
	if nickname != a.nicknameFromCookie(r) {
		a.setNicknameCookie(w, r, nickname)
	}
}

func generateNickname() (result string) {
	var b [3]byte
	_, _ = rand.Read(b[:])
	result = fmt.Sprintf("Player%X", b)
	return
}

func (a *App) setNickname(player *game.Player, nickname string) {
	if player == nil {
		return
	}
	nickname = game.NormalizeNickname(nickname)
	if room := player.Room; room != nil {
		room.SetNickname(player, nickname)
		return
	}
	player.Nickname = nickname
	player.NicknameInput = nickname
}

func (a *App) createRoom(player *game.Player) (result1 *game.Room, errResult error) {
	room, err := a.Manager.CreateRoom(player, a.Catalog.DefaultDeckIDs())
	if err != nil {
		result1, errResult = nil, err
		return
	}
	a.Jaws.Dirty(a.Manager, room)
	result1, errResult = room, nil
	return
}

func (a *App) joinRoom(player *game.Player, roomCode string) (result1 *game.Room, errResult error) {
	room, err := a.Manager.JoinRoom(roomCode, player)
	if err != nil {
		result1, errResult = nil, err
		return
	}
	a.Jaws.Dirty(a.Manager, room, player)
	result1, errResult = room, nil
	return
}

func (a *App) leaveRoom(player *game.Player) (result *game.Room) {
	result, _ = a.Manager.LeaveRoom(player)
	a.Jaws.Dirty(a.Manager, result, player)
	return
}

func (a *App) roomURL(code string) (result string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		result = "/"
		return
	}
	result = path.Join("/room", code)
	return
}

func (a *App) RoomURL(code string) (result string) { result = a.roomURL(code); return }

func requestIsSecure(r *http.Request) (result bool) {
	if r == nil {
		result = false
		return
	}
	if r.TLS != nil {
		result = true
		return
	}
	proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	result = strings.EqualFold(proto, "https")
	return
}

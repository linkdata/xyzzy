package ui

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"html/template"
	"net/http"
	"path"
	"strings"

	"github.com/linkdata/jaws"
	jui "github.com/linkdata/jaws/lib/ui"
	"github.com/linkdata/staticserve"
	"github.com/linkdata/xyzzy"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
)

const (
	sessionKeyNickname = "nickname"
	sessionKeyPlayerID = "player_id"
	sessionKeyRoomCode = "room_code"
	nicknameCookieTTL  = 365 * 24 * 60 * 60
)

type App struct {
	Jaws    *jaws.Jaws
	Catalog *deck.Catalog
	Manager *game.Manager
}

func New(jw *jaws.Jaws, catalog *deck.Catalog, manager *game.Manager) *App {
	return &App{Jaws: jw, Catalog: catalog, Manager: manager}
}

func (a *App) SetupRoutes(mux *http.ServeMux) error {
	templates, err := template.New("root").Funcs(template.FuncMap{
		"cardhtml": formatCardHTML,
		"join":     strings.Join,
	}).ParseFS(xyzzy.Assets, "assets/templates/*.html")
	if err != nil {
		return err
	}
	if err := a.Jaws.AddTemplateLookuper(templates); err != nil {
		return err
	}
	if err := a.Jaws.Setup(mux.Handle, "/static",
		staticserve.MustNewFS(xyzzy.Assets, "assets/static", "app.css", "images/favicon.svg"),
	); err != nil {
		return err
	}
	mux.Handle("GET /jaws/", a.Jaws)
	mux.Handle("GET /", http.HandlerFunc(a.serveLobby))
	mux.Handle("GET /room/{code}", http.HandlerFunc(a.serveRoom))
	return nil
}

func (a *App) Middleware(next http.Handler) http.Handler {
	return a.Jaws.Session(a.Jaws.SecureHeadersMiddleware(next))
}

func (a *App) serveLobby(w http.ResponseWriter, r *http.Request) {
	sess := a.session(r)
	a.syncNicknameCookie(w, r, sess)
	page := NewLobbyPage(a, sess)
	a.reconcileSession(page.Session)
	if err := a.renderTemplate(w, r, "index.html", page); err != nil {
		a.Jaws.Log(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *App) serveRoom(w http.ResponseWriter, r *http.Request) {
	sess := a.session(r)
	a.syncNicknameCookie(w, r, sess)
	page := NewRoomPage(a, sess, r.PathValue("code"))
	a.reconcileSession(page.Session)
	if current := a.roomCode(page.Session); current != "" && current != page.RoomCode {
		http.Redirect(w, r, "/room/"+current, http.StatusSeeOther)
		return
	}
	if page.Nickname() != "" && a.roomCode(page.Session) == "" {
		if _, err := a.joinRoom(page.Session, page.RoomCode); err != nil {
			if err == game.ErrRoomNotFound {
				page.Alert = ""
			} else {
				page.Alert = err.Error()
			}
		}
	}
	if err := a.renderTemplate(w, r, "room.html", page); err != nil {
		a.Jaws.Log(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *App) renderTemplate(w http.ResponseWriter, r *http.Request, name string, dot any) error {
	req := a.Jaws.NewRequest(r)
	return req.NewElement(jui.Template{Name: name, Dot: dot}).JawsRender(w, nil)
}

func (a *App) session(r *http.Request) *jaws.Session {
	sess := a.Jaws.GetSession(r)
	if sess == nil {
		panic("ui.App handlers require JaWS session middleware")
	}
	return sess
}

func (a *App) Dirty(tags ...any) {
	var filtered []any
	for _, tag := range tags {
		if tag != nil {
			filtered = append(filtered, tag)
		}
	}
	if len(filtered) > 0 {
		a.Jaws.Dirty(filtered...)
	}
}

func (a *App) DirtyLobby() {
	a.Dirty(a.Manager)
}

func (a *App) DirtyRoom(room *game.Room) {
	a.Dirty(a.Manager, room)
}

func (a *App) sessionString(sess *jaws.Session, key string) string {
	if value, ok := sess.Get(key).(string); ok {
		return value
	}
	return ""
}

func (a *App) nicknameCookieName() string {
	name := strings.TrimSpace(a.Jaws.CookieName)
	if name == "" {
		name = "jaws"
	}
	return name + "_nickname"
}

func (a *App) nicknameFromCookie(r *http.Request) string {
	cookie, err := r.Cookie(a.nicknameCookieName())
	if err != nil || cookie.Value == "" {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
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

func (a *App) syncNicknameCookie(w http.ResponseWriter, r *http.Request, sess *jaws.Session) {
	cookieName := a.nicknameFromCookie(r)
	sessionName := a.nickname(sess)
	if sessionName == "" && cookieName != "" {
		a.setSessionString(sess, sessionKeyNickname, cookieName)
		sessionName = cookieName
	}
	if sessionName != "" && sessionName != cookieName {
		a.setNicknameCookie(w, r, sessionName)
	}
}

func (a *App) setSessionString(sess *jaws.Session, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		sess.Set(key, nil)
		return
	}
	sess.Set(key, value)
}

func (a *App) nickname(sess *jaws.Session) string { return a.sessionString(sess, sessionKeyNickname) }
func (a *App) playerID(sess *jaws.Session) string { return a.sessionString(sess, sessionKeyPlayerID) }
func (a *App) roomCode(sess *jaws.Session) string { return a.sessionString(sess, sessionKeyRoomCode) }

func (a *App) setNickname(sess *jaws.Session, nickname string) {
	a.setSessionString(sess, sessionKeyNickname, nickname)
}
func (a *App) setRoomCode(sess *jaws.Session, roomCode string) {
	a.setSessionString(sess, sessionKeyRoomCode, strings.ToUpper(roomCode))
}

func (a *App) ensurePlayerID(sess *jaws.Session) string {
	if id := a.playerID(sess); id != "" {
		return id
	}
	var raw [8]byte
	_, _ = rand.Read(raw[:])
	id := hex.EncodeToString(raw[:])
	a.setSessionString(sess, sessionKeyPlayerID, id)
	return id
}

func (a *App) clearRoom(sess *jaws.Session) {
	sess.Set(sessionKeyRoomCode, nil)
}

func (a *App) reconcileSession(sess *jaws.Session) {
	roomCode := a.roomCode(sess)
	playerID := a.playerID(sess)
	if roomCode == "" || playerID == "" {
		a.clearRoom(sess)
		return
	}
	if !a.Manager.ReconcileMembership(roomCode, playerID) {
		a.clearRoom(sess)
	}
}

func (a *App) createRoom(sess *jaws.Session) (*game.Room, error) {
	nickname := strings.TrimSpace(a.nickname(sess))
	if nickname == "" {
		return nil, game.ErrNeedNickname
	}
	playerID := a.ensurePlayerID(sess)
	room, err := a.Manager.CreateRoom(playerID, nickname, a.Catalog.DefaultDeckIDs())
	if err != nil {
		return nil, err
	}
	a.setRoomCode(sess, room.Code())
	a.DirtyRoom(room)
	return room, nil
}

func (a *App) joinRoom(sess *jaws.Session, roomCode string) (*game.Room, error) {
	nickname := strings.TrimSpace(a.nickname(sess))
	if nickname == "" {
		return nil, game.ErrNeedNickname
	}
	playerID := a.ensurePlayerID(sess)
	room, err := a.Manager.JoinRoom(roomCode, playerID, nickname)
	if err != nil {
		return nil, err
	}
	a.setRoomCode(sess, room.Code())
	a.DirtyRoom(room)
	return room, nil
}

func (a *App) leaveRoom(sess *jaws.Session) *game.Room {
	roomCode := a.roomCode(sess)
	playerID := a.playerID(sess)
	if roomCode == "" || playerID == "" {
		a.clearRoom(sess)
		return nil
	}
	room, _ := a.Manager.LeaveRoom(roomCode, playerID)
	a.clearRoom(sess)
	a.DirtyRoom(room)
	return room
}

func (a *App) roomURL(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return "/"
	}
	return path.Join("/room", code)
}

func (a *App) RoomURL(code string) string { return a.roomURL(code) }

func requestIsSecure(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	return strings.EqualFold(proto, "https")
}

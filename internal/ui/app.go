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

func New(jw *jaws.Jaws, catalog *deck.Catalog, manager *game.Manager) *App {
	return &App{Jaws: jw, Catalog: catalog, Manager: manager}
}

func (a *App) SetupRoutes(mux *http.ServeMux) error {
	templates, err := template.New("root").Funcs(template.FuncMap{
		"blackFootnote":   renderBlackCardFootnote,
		"cardAction":      a.CardAction,
		"cardBody":        a.HandCardHTML,
		"cardAttrs":       a.HandCardAttrs,
		"cardClass":       a.HandCardClass,
		"createRoomClick": a.CreateRoomClick,
		"deckToggle":      a.DeckToggle,
		"deckToggleAttrs": a.DeckToggleAttrs,
		"join":            strings.Join,
		"lobbyMain":       a.LobbyMain,
		"lobbySidebar":    a.LobbySidebar,
		"orderedDecks":    a.Catalog.OrderedDecks,
		"onlinePlayers":   a.Jaws.SessionCount,
		"playerHost":      func(room *game.Room, player *game.Player) bool { return room != nil && room.IsHost(player) },
		"playerJudge":     func(room *game.Room, player *game.Player) bool { return room != nil && room.IsJudge(player) },
		"playerScore": func(room *game.Room, player *game.Player) int {
			if room == nil {
				return 0
			}
			return room.ScoreFor(player)
		},
		"playerSubmitted": func(room *game.Room, player *game.Player) bool {
			return room != nil && room.SubmittedBy(player)
		},
		"publicRooms":       a.Manager.PublicRooms,
		"roomByCode":        a.Manager.Room,
		"roomMain":          a.RoomMain,
		"roomSidebar":       a.RoomSidebar,
		"saveNicknameClick": a.SaveNicknameClick,
		"stateBadgeClass":   stateBadgeClass,
		"submissionAttrs":   a.SubmissionAttrs,
		"submissionAction":  a.SubmissionAction,
		"submissionClass":   a.SubmissionClass,
		"submissionBody":    a.SubmissionHTML,
		"waitingDetail":     waitingDetail,
		"waitingTitle":      waitingTitle,
		"whiteFootnote":     renderWhiteCardFootnote,
	}).ParseFS(xyzzy.Assets, "assets/templates/*.html")
	if err != nil {
		return err
	}
	if err := a.Jaws.AddTemplateLookuper(templates); err != nil {
		return err
	}
	if err := a.Jaws.Setup(mux.Handle, "/static",
		jawsboot.Setup,
		staticserve.MustNewFS(xyzzy.Assets, "assets/static", "images/favicon.svg", "app.css"),
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
	player := a.player(sess, r)
	a.cleanupExpired()
	if player.Room != nil {
		a.leaveRoom(player)
	}
	a.syncNicknameCookie(w, r, player)
	if err := a.renderTemplate(w, r, "index.html", player); err != nil {
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
	if err := a.renderTemplate(w, r, "room.html", player); err != nil {
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

func (a *App) player(sess *jaws.Session, r *http.Request) *game.Player {
	if player, ok := sess.Get(sessionKeyPlayer).(*game.Player); ok && player != nil {
		if player.Session == nil {
			player.Session = sess
		}
		if player.Nickname == "" {
			player.Nickname = generateNickname()
		}
		if player.NicknameInput == "" {
			player.NicknameInput = player.Nickname
		}
		return player
	}
	nickname := a.nicknameFromCookie(r)
	if nickname == "" {
		nickname = generateNickname()
	} else {
		nickname = game.NormalizeNickname(nickname)
	}
	player := &game.Player{
		Session:       sess,
		Nickname:      nickname,
		NicknameInput: nickname,
	}
	sess.Set(sessionKeyPlayer, player)
	return player
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

func (a *App) nicknameCookieName() string {
	name := strings.TrimSpace(a.Jaws.CookieName)
	if name == "" {
		name = "jaws"
	}
	return name + "_nickname"
}

func (a *App) nicknameFromCookie(r *http.Request) string {
	if r == nil {
		return ""
	}
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

func generateNickname() string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("Player%X", b)
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

func (a *App) createRoom(player *game.Player) (*game.Room, error) {
	room, err := a.Manager.CreateRoom(player, a.Catalog.DefaultDeckIDs())
	if err != nil {
		return nil, err
	}
	a.Jaws.Dirty(a.Manager, room, player)
	return room, nil
}

func (a *App) joinRoom(player *game.Player, roomCode string) (*game.Room, error) {
	room, err := a.Manager.JoinRoom(roomCode, player)
	if err != nil {
		return nil, err
	}
	a.Jaws.Dirty(a.Manager, room, player)
	return room, nil
}

func (a *App) leaveRoom(player *game.Player) *game.Room {
	room, _ := a.Manager.LeaveRoom(player)
	a.Jaws.Dirty(a.Manager, room, player)
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

func stateBadgeClass(state game.RoomState) string {
	switch state {
	case game.StateLobby:
		return "bg-secondary"
	case game.StatePlaying:
		return "bg-success"
	default:
		return "bg-warning text-dark"
	}
}

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

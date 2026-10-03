[![build](https://github.com/linkdata/xyzzy/actions/workflows/build.yml/badge.svg)](https://github.com/linkdata/xyzzy/actions/workflows/build.yml)
[![coverage](https://github.com/linkdata/xyzzy/blob/gitcoverage/main/badge.svg)](https://html-preview.github.io/?url=https://github.com/linkdata/xyzzy/blob/gitcoverage/main/report.html)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/linkdata/xyzzy/badge)](https://scorecard.dev/viewer/?uri=github.com/linkdata/xyzzy)

# Pretend You're Xyzzy

Pretend You're Xyzzy is a multiplayer fill-in-the-blank party card game and a
complete example of a server-driven UI built with
[JaWS](https://github.com/linkdata/jaws). Go templates compose standard JaWS
widgets with synchronized game state, bindings, and actions. JaWS carries
browser events and targeted DOM updates over WebSocket. Bootstrap and the JaWS
transport run in the browser; there is no custom application JavaScript.

The [JaWS v0.807.0 guide](https://github.com/linkdata/jaws/blob/v0.807.0/doc/README.md)
matches the framework version selected in [go.mod](go.mod).

## Run it

The module requires Go 1.27 or later. To start a local two-player game:

```sh
go run ./cmd/xyzzy -debug -address 127.0.0.1:8080
```

Open <http://127.0.0.1:8080/> once in a regular window and once in a private
window, or use two separate browser profiles. A single browser profile shares
one JaWS Session and therefore represents one player. Debug mode enables JaWS
debugging, lowers the minimum player count from three to two, permits a target
score of one, and starts with the highest-pick prompt.

All templates, styles, and card data are embedded in the binary. There is no
database, Node.js build, or npm dependency. A production binary can be built
with:

```sh
go build -o xyzzy ./cmd/xyzzy
```

Game and session state are in memory, so restarting the process starts fresh.
Use `-certdir` with `fullchain.pem` and `privkey.pem` for direct HTTPS. Run
`go run ./cmd/xyzzy -h` for all server options and their defaults.

### Reverse proxies

When TLS terminates at a reverse proxy, use `-trust-forwarded-headers` so JaWS
uses the forwarded client IP for request and session binding and the forwarded
scheme for secure cookies and WebSocket origin checks. Enable this only behind
one controlled proxy that removes client-supplied forwarding headers and sets
the client IP and scheme itself. Leave it disabled when xyzzy is directly
reachable. The proxy must preserve the public host for WebSocket origin checks
and provide HSTS when it serves HTTPS.

Room creation is rate-limited by the client IP stored in the JaWS Session,
using the same forwarding-header trust setting.

## How the UI works

### Immediate-mode ownership

Each page request creates a request-specific `ui.Handler` and render dot. The
render code reads the current synchronized state and describes the UI that
should exist now. JaWS owns the live `Request` and `Element` trees, retains the
definitions needed for events and updates, and reruns selected definitions.
When a child list can change, a `Container` reconstructs and reconciles only its
direct children.

Values such as `templateDot`, `roomSection`, `whiteCardView`, and
`submissionView` contain stable pointers to game state. JaWS retains these
definitions with its live Elements. `internal/ui` integrates HTTP, sessions,
and templates; `internal/game` keeps bindings and actions beside the state they
operate on. Initial rendering and live updates use the same getters.

### Templates and widgets

The templates use each live primitive for one distinct job:

| Need | Primitive | Example in this repository |
| --- | --- | --- |
| Render a full document | `ui.Handler` | `serveLobby` and `serveRoom` |
| Run behavior when the JaWS client connects | `jaws.ConnectHandler` | `roomPageDot` joins the requested room |
| Include structure without a live wrapper | standard Go `{{template ...}}` | the head, nickname modal, and shared black-card markup |
| Rerender a fixed live region | `ui.Template` via `$.Template` or `ui.NewTemplate` | the lobby sidebar and current room-state panel |
| Change direct child identity or presence | `ui.Container` via `$.Container` | `roomSection` selects the sidebar and main room child |
| Edit an addressable scalar | `bind.New` | nickname, privacy, and target score |
| Describe a semantic action | `ui.Object` | create, start, submit, judge, and review actions |
| Display dynamic text | a bound `Span` or `Button` getter | the navbar nickname and shared review countdown |

Standard Go `if`, `with`, and `range` handle render-time conditions and iteration.
A retained `Template` owns a stable live wrapper whose inner content JaWS may
replace. A `Container` is used where the set or identity of direct children can
change. `roomSection` is a comparable value that constructs fresh
`Template` children; equal child definitions let JaWS retain the existing child
`Element`.

The main `roomSection` selects `room_game_lobby.html`,
`room_game_playing.html`, `room_game_judging.html`, or
`room_game_review.html` in Go. A state transition changes the `Container` child
identity; a same-state update reconstructs an equal `Template` value and keeps
its existing `Element`. Each state template retains only stable `App`, `Room`,
and `Player` pointers and calls synchronized state getters directly.
Addressable scalar controls bind the real field; methods such as `HandFor` and
`Submissions` copy mutable slices while holding the room lock before returning
them. The small card definitions likewise retain only the pointers needed to
read current state and handle an event.

### Definition equality and dependency tags

Immediate-mode reconciliation answers two different questions:

1. Is this newly described child equal to the retained child?
2. Which retained Elements depend on a piece of state that changed?

Comparable UI definitions answer the first question. Stable dependency tags
answer the second. Dirtying a Container's dependency tag reconciles its child
list; equal retained children need their own tags for content updates. Card
Templates register selection tags for each player and card or submission.
Selection changes refresh affected cards' content and pressed state while
retaining their button nodes. Action Buttons use getters and separate readiness
tags to update their disabled state.

| Dependency tag or accessor | Typical dependents |
| --- | --- |
| `*game.Manager` | the public room list and room lookup |
| `*game.Player` | displayed nickname, room membership, and player-specific branches |
| `*game.Room` | shared room summary and game regions |
| `roomDeckTag{Room, Deck}` | that deck's checkboxes in one room |
| `room.DeckSelectionTag()` | selected-card counts and Start button readiness |
| `room.HandCardTag(player, card)` | that player's selection order and pressed state for one card |
| `room.SubmissionTag(player, submission)` | that player's selection state for one submission |
| `player.HandReadinessTag()` / `player.JudgeReadinessTag()` | Submit / Judge button readiness |
| `&player.NicknameInput` | nickname inputs across that player's pages |
| field pointers such as `&r.targetScore` | independently bound controls and labels |
| `&r.reviewDeadline` | the judge's countdown button and every other player's countdown span |
| `Jaws.ActiveSessionCountTag()` | the live lobby presence count |

Calling `Dirty(tag)` updates matching Elements across Requests. `Dirty(elem)`
selects only that Element. An Element can register multiple independent tags;
dirtying one does not dirty the others. The same field tag can connect different
widget types: the target score connects a range input and badge, and the review
deadline connects a button and text span.

### Bindings and actions

Simple fields use `bind.New(&lock, &field)`. That supplies synchronized storage
and a stable field-pointer dependency tag without another adapter type.
Validation is added with `SetLocked`, which checks the current room state while
the same lock is held and then delegates to the original binder.

Deck selection uses a standard `Checkbox` with a `deckInput` binding that reads
and changes membership in the room's selected-deck set. Its
`roomDeckTag{Room, Deck}` dependency scopes input reconciliation to one deck;
an actual selection change also dirties `Room.DeckSelectionTag()` for the count
Span and Start button readiness. The remaining controls stay intact. An unchanged
edit returns `jaws.ErrValueUnchanged` and dirties neither.

`Manager.SetNickname` normalizes a saved nickname and, when the player is seated,
makes it unique within the room. It dirties only changed dependencies after unlocking:
Player and Room tags for displayed-name changes, the field tag for input
corrections, and the Manager tag for public-host name changes.

`Manager.CreateRoom` and `Manager.JoinRoom` seat the player under the normalized,
room-unique nickname draft, even if it was not saved. If the stored input changes,
they dirty its field tag after unlocking. Existing nickname inputs, including
those on sibling pages, then receive the accepted value.

Actions use `ui.Object` to combine getters, click handlers, and initial
attributes. Templates select standard widgets, static attributes, and additional
dependency tags. For example, the lobby renders its Start button with:

```gotemplate
{{with .Dot}}
  {{$.Button (.Room.StartGameButton .Player) `class="btn btn-success"` .Room.DeckSelectionTag}}
{{end}}
```

Static attributes stay in template strings. Dynamic initial attributes come
from methods such as `LobbyControlAttrs` and card `InitialAttrs`, which return
`template.HTMLAttr`. Getters and template methods update attributes afterward;
this application defines no custom `JawsRender` or `JawsUpdate` methods.

Rendered `hidden` and `disabled` attributes are presentation, not
authorization. Privileged room mutations revalidate the player's permissions
and current room state.

### The countdown is server-driven dynamic text

The judge's `ReviewButton` and other players' `ReviewStatus` Span read the room's
deadline under its lock. Both register `&r.reviewDeadline`. One room timer dirties
that tag at displayed-second boundaries, updating both widget types across
Requests. At the deadline, it advances the state and dirties the Room tag so
the containing region updates.

### Sessions and initial rendering

`SessionMiddleware` establishes the Session; the page handler obtains its
`Player` before `ui.Handler` performs the initial render. A JaWS Session identifies
the player; an independent HttpOnly cookie restores only the nickname after an
ephemeral Session expires. `App.player` serializes the Session's get-or-create
operation, so concurrent page requests cannot create different players for one
Session.

`GET /room/{code}` does not take a seat. Its top-level `roomPageDot` implements
`jaws.ConnectHandler`, so a new viewer attempts to join only after JaWS accepts
the page's WebSocket. The handler shares one captured room identity with both
room sections and reloads if that identity changes before connection. A
successful join dirties the Manager, Room, and Player tags; Containers then
reconcile into the seated UI. Any normalized nickname input is corrected through
its field tag. A GET alone cannot fill a room.

The lobby binds a `Span` directly to `Jaws.ActiveSessionCount` using
`ActiveSessionCountTag`. JaWS dirties the tag as request and session activity
changes, so the count reconciles live. It is an intentionally approximate
indicator of distinct active Sessions. Tabs sharing a Session count once;
GET-only and disconnected Sessions do not count.

Visiting `GET /` likewise leaves the player's current room before rendering the
lobby.

### State and concurrency

State types own their synchronization. Mutations validate under their state
locks and notify JaWS after releasing them. Accessors copy mutable collections
before returning, and templates never retain a room lock across execution.
Separate getter calls may observe adjacent valid states, but every read is
race-free and the relevant dirty notification converges the display.

## Deliberate boundaries

- State is process-local and is not shared across server replicas or persisted
  across restarts.
- New rooms are private. The host can publish one before starting the game.
  Private rooms are omitted from the public list, but their URLs are not an
  authorization boundary. A code remains valid after a public room is made
  private again.
- A room page tries to join once, when its JaWS connection starts. A seat that
  opens before that connection is accepted can be claimed; a seat that opens
  after a failed attempt requires a reload.
- The first lobby or room visit from an anonymous browser creates an ephemeral
  Session and player. Expired seated players are removed from rooms during
  later page requests; an unseated player has the JaWS Session's lifetime.
- Session-expiry cleanup scans all in-memory rooms during page requests, and
  lobby validation recomputes selected-deck unions. Both assume demo-scale room
  and deck counts.
- JaWS-managed buttons and inputs require the WebSocket connection. The server
  does not replay actions performed while a browser is offline.

## Code map

- [`cmd/xyzzy/main.go`](cmd/xyzzy/main.go) assembles the catalog, JaWS server,
  game manager, and HTTP server.
- [`internal/ui/app.go`](internal/ui/app.go) defines routes, sessions, and
  full-page handlers.
- [`internal/ui/section.go`](internal/ui/section.go) contains the comparable
  `Container` definitions.
- [`internal/ui/template_dot.go`](internal/ui/template_dot.go) contains
  request dots and small template adapter definitions.
- [`internal/deck/catalog.go`](internal/deck/catalog.go) loads the immutable
  embedded card and deck catalog.
- [`internal/game/jaws_ui.go`](internal/game/jaws_ui.go) keeps synchronized
  binders, dynamic getters, and semantic controls beside their state.
- [`internal/game/selection_ui.go`](internal/game/selection_ui.go) and
  [`internal/game/deck_ui.go`](internal/game/deck_ui.go) define selection and
  readiness tags and deck counts.
- [`internal/game/manager.go`](internal/game/manager.go) coordinates room
  membership, nickname normalization, and session-expiry cleanup.
- [`internal/game/room.go`](internal/game/room.go) contains the game state
  machine and review timer.
- [`cmd/importpyx/main.go`](cmd/importpyx/main.go) imports PYX SQL data into the
  tracked card assets.
- [`assets/templates`](assets/templates) defines the HTML shape.
- [`internal/ui/immediate_mode_live_test.go`](internal/ui/immediate_mode_live_test.go)
  exercises multi-browser reconciliation through real JaWS WebSockets;
  [`nickname_seating_test.go`](internal/ui/nickname_seating_test.go) covers
  nickname correction during connection and on sibling pages.

## Verify it

Run these checks from the module root. The external linters (`staticcheck`,
`golangci-lint`, and `gosec`) need installation; their versions are pinned in
the [build workflow](.github/workflows/build.yml).

```sh
test -z "$(gofmt -l .)"
go mod tidy -diff
go vet ./...
staticcheck ./...
golangci-lint run
gosec ./...
go test -race ./...
go test ./...
go build ./...
```

The race-enabled run checks concurrent state and multi-client updates. The
plain run also compiles and exercises JaWS's release tag implementation; JaWS
uses a different, debug-friendly implementation under `-race`.

## License

Pretend You're Xyzzy is distributed under the [MIT License](LICENSE).

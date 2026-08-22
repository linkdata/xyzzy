# Pretend You're Xyzzy

Pretend You're Xyzzy is a multiplayer fill-in-the-blank party card game and a
complete example of a server-driven UI built with
[JaWS](https://github.com/linkdata/jaws). Synchronized Go state is the source of
truth, templates project that state, and JaWS carries browser events and
targeted DOM updates over WebSocket.

JaWS is an immediate-mode framework, not an MVC framework. This application
does not have controllers, retained view models, or application-defined
browser state. It also has no custom application JavaScript: Bootstrap and the
JaWS browser transport run in the browser, while the game and its UI behavior
remain in Go.

## Run it

The module requires Go 1.26.1 or later. To start a local two-player game:

```sh
go run ./cmd/xyzzy -debug -address 127.0.0.1:8080
```

Open <http://127.0.0.1:8080/> once in a regular window and once in a private
window, or use two separate browser profiles. A single browser profile shares
one JaWS session and therefore represents one player. Debug mode lowers the
normal minimum player count from three to two and makes short games easier to
exercise.

All templates, styles, and card data are embedded in the binary. There is no
database, Node.js build, or npm dependency. A production binary can be built
with:

```sh
go build -o xyzzy ./cmd/xyzzy
```

Game and session state are in memory, so restarting the process starts fresh.

## Design rationale

### Immediate-mode ownership

Each page request creates a request-specific `ui.Handler` and render dot. The
render code reads the current synchronized state and describes the UI that
should exist now. JaWS owns the live `Request` and `Element` trees, retains the
definitions needed for events and updates, and reruns selected definitions.
When a child list can change, a `Container` reconstructs and reconciles only its
direct children.

The application never stores a JaWS `Request`, `Element`, or UI tree in a
`Manager`, `Room`, or `Player`. Values such as `templateDot`, `roomSection`,
`whiteCardView`, and `submissionView` are short-lived definitions containing
stable pointers to authoritative state, not controllers or retained view
models.

The package boundary follows ownership and lifetime rather than MVC roles.
`internal/ui` integrates HTTP, sessions, and templates; `internal/game/jaws_ui.go`
places synchronized bindings and actions beside the state they operate on.

Initial HTML and live updates come from the same Go definitions and getters;
there is no separately maintained client-side rendering path.

### Choose the smallest JaWS primitive

The templates use each live primitive for one distinct job:

| Need | Primitive | Example in this repository |
| --- | --- | --- |
| Render a full document | `ui.Handler` | `serveLobby` and `serveRoom` |
| Run behavior when the JaWS client connects | `jaws.ConnectHandler` | `roomPageDot` joins the requested room |
| Include static structure | native `{{template ...}}` | the head, nickname modal, and shared black-card markup |
| Rerender a fixed live region | `ui.Template` via `$.Template` or `ui.NewTemplate` | the lobby sidebar and current room-state panel |
| Change direct child identity or presence | `ui.Container` via `$.Container` | `roomSection` selects the sidebar and main room child |
| Edit an addressable scalar | `bind.New` | nickname, privacy, and target score |
| Describe a semantic action | `ui.Object` | create, start, submit, judge, and review actions |
| Display dynamic text | a bound `Span` or `Button` getter | the navbar nickname and shared review countdown |

Native template inclusion is enough for static markup. A retained `Template`
owns a stable live wrapper whose inner content JaWS may replace. A `Container`
is used only where the set or identity of direct children can change. In
particular, `roomSection` is a comparable value that constructs fresh
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

Template execution is deliberately not a transaction over the whole room. If
a mutation lands between two getter calls, one render may combine observations
from adjacent valid states. Every read remains race-free, actions revalidate
against current state, and the UI path that commits a mutation publishes the
relevant `Room` or `Player` tag so JaWS converges the retained element tree.
This keeps the immediate-mode projection direct instead of introducing a
screen-shaped DTO.

The lobby's online count is a bound `Span` getter over
`Jaws.ActiveSessionCount`. The application constructor enables
`StatusMetricActiveSessions`, and the getter registers
`ActiveSessionCountTag`, so JaWS dirties the span when its maintenance loop
observes a different count. Multiple connected tabs sharing one session still
count once.

This keeps wrapper ownership unambiguous: the template emits the contents of a
JaWS wrapper, not a competing copy of the wrapper itself.

### Definition equality and dependency tags are separate

Immediate-mode reconciliation answers two different questions:

1. Is this newly described child equal to the retained child?
2. Which retained elements depend on a piece of state that changed?

Comparable UI definitions answer the first question. Stable dependency tags
answer the second. A parent becoming dirty does not imply that every equal
retained child will rerender, so live children register the state they actually
read.

| Dependency tag | Typical dependents |
| --- | --- |
| `*game.Manager` | the public room list and unseated room lookup |
| `*game.Player` | room membership and player-specific branches |
| `*game.Room` | shared room summary and game regions |
| `roomDeckTag{Room, Deck}` | that deck's checkboxes in one room |
| field pointers such as `&r.targetScore` | independently bound controls and labels |
| `&r.reviewDeadline` | the judge's countdown button and every other player's countdown span |
| `Jaws.ActiveSessionCountTag()` | the live lobby presence count |

Calling `Dirty` with one of these tags fans the update out to matching elements
in every live JaWS request. Dirtying an element itself is request-local. The
same field tag can deliberately connect different widget types, as the target
score connects a range input and badge, and the review deadline connects a
button and text span.

### Keep bindings and actions beside authoritative state

Simple fields use `bind.New(&lock, &field)`. That supplies synchronized storage
and a stable field-pointer dependency tag without another adapter type.
Validation is added with `SetLocked`, which checks the current room state while
the same lock is held and then delegates to the original binder.

Deck selection is the one custom input definition because “is this deck
enabled?” is computed from a set rather than stored in an addressable scalar.
`deckInput` implements `JawsGet`, `JawsSet`, and `JawsGetTag` directly. Its
`roomDeckTag{Room, Deck}` dependency lets a rejected browser edit reconcile that
deck's checkbox in every live request for the room. Unchanged peers suppress the
wire update, so normally only the originating input is corrected. An actual
selection change separately dirties the `Room`, after the room lock is released,
so shared counts and controls update in every live request. An unchanged edit
returns `jaws.ErrValueUnchanged` and dirties neither.

`Manager.SetNickname` is likewise the single committed-nickname boundary. It
normalizes or uniquifies the value under the state locks and, when either stored
field changes, releases them before publishing the manager, player, room, and
exact nickname-field dependency. A normalized save therefore reconciles sibling
inputs and navbar labels as well as shared room text, while a canonical no-op
publishes nothing.

The navbar's Bootstrap modal trigger remains ordinary HTML around the bound
nickname `Span`. The span uses Bootstrap's `pe-none` utility so pointer clicks
target the outer button and reach Bootstrap's document-level modal handler;
JaWS can still update the span's text through its element ID.

Semantic actions are returned as `ui.Object` values. The object's primary
getter may be a dynamic string getter, so its label, click behavior, dependency
tag, and initial attributes stay together. Templates can therefore say, for
example, “render the room's start button” rather than assemble one action from
separate label, attribute, and callback helpers.

Rendered `hidden` and `disabled` attributes are presentation, not
authorization. Privileged room mutations revalidate the player's permissions
and current room state.

`LobbyControlAttrs` deliberately stays separate from the target-score binder.
The range input needs `disabled`, while the span displaying the same bound value
does not. Binder attribute hooks also run while the binder lock is held, so an
attribute helper that reacquired the room lock could deadlock when a writer is
waiting.

### The countdown is server-driven dynamic text

The review countdown demonstrates why dynamic button text belongs in JaWS:

1. `ReviewTitle` copies the winner's nickname string while holding the room read
   lock. `ReviewStatus` and `ReviewButton` choose the viewer's role under the
   room lock; their dynamic getter later reads the current deadline and outcome
   under a fresh room read lock whenever JaWS invokes it.
2. The judge receives a dynamic JaWS `Button`; other players receive a dynamic
   JaWS `Span`.
3. Both getters register the same `&r.reviewDeadline` dependency tag.
4. One room timer dirties that tag at displayed-second boundaries, updating
   both widget types in every connected browser.
5. At the deadline, the timer advances the state and dirties the room so the
   containing live region changes.

There is no per-browser timer, custom event protocol, `MutationObserver`, or
application JavaScript. Each getter computes role-appropriate display text from
the authoritative server deadline, and JaWS transports targeted DOM operations.

### Sessions and initial rendering

`SessionMiddleware` wraps the page handlers, so a `Player` exists before
`ui.Handler` performs the initial render. A JaWS session identifies the player;
an independent HttpOnly cookie restores only the nickname after an ephemeral
session expires. `App.player` serializes the session's get-or-create operation,
so concurrent page requests cannot create different players for one session.

`GET /room/{code}` does not take a seat. Its top-level `roomPageDot` implements
`jaws.ConnectHandler`, so the initial document describes the unseated state and
the join runs only after JaWS accepts the page's WebSocket. The handler resolves
the route to one stable room identity shared by the connect hook and both room
sections. If that identity disappears or changes before connection, the page
reloads instead of attaching a replacement room to retained definitions for the
old one. A successful join dirties the existing `Player`, `Room`, and room-list
dependencies; the retained sidebar and main `Container` values then reconcile
into the seated UI. Plain crawlers, prefetchers, and link unfurlers therefore
cannot fill a room by fetching its URL. A client that runs the JaWS script and
opens the WebSocket can still join, because connection is the intended lifecycle
boundary rather than proof of human intent.

Visiting `GET /` likewise leaves the player's current room before rendering the
lobby.

### Concurrency without retained presentation state

State types own their synchronization. Mutation methods validate and update
under the relevant locks. The UI, timer, or session path that commits the
change notifies JaWS after releasing them. Every template-facing room accessor
takes the room lock when it reads mutable state, and collection accessors return
copies rather than exposing mutable slice storage. Templates never retain a
room lock across execution. Accessors and mutations release it before JaWS
notification; `SetLocked` binder hooks are the deliberate exception that
validate and write while the binder already holds it.

A state template can therefore observe adjacent valid states across separate
getter calls, but it cannot race on the underlying data. A subsequent relevant
dirty notification converges the display. Values needed after unlocking are
copied as primitives: `ReviewTitle`, for example, copies the winner's nickname
string rather than retaining a `*Player` field read. Countdown getters capture
no room state; they read the current deadline and outcome when JaWS invokes
them and remain tagged on the authoritative deadline field.

## Deliberate boundaries

- State is process-local and is not shared across server replicas or persisted
  across restarts.
- A private room is omitted from the public list; possession of its URL is the
  access mechanism, not an authorization boundary.
- A room page tries to join once, when its JaWS connection starts. A seat that
  opens before that connection is accepted can be claimed; a seat that opens
  after a failed attempt requires a reload.
- The first lobby or room visit from an anonymous browser creates an ephemeral
  session and player. Expired seated players are removed from rooms during
  later page requests; an unseated player has the JaWS session's lifetime.
- The lobby's live presence count uses distinct sessions attached to active
  JaWS Requests. Connected tabs sharing one session count once, while GET-only
  and disconnected sessions do not count. Maintenance sampling may coalesce
  intermediate changes, so the display can lag briefly.
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
- [`internal/game/jaws_ui.go`](internal/game/jaws_ui.go) keeps synchronized
  binders, dynamic getters, and semantic controls beside their state.
- [`internal/game/room.go`](internal/game/room.go) contains the game state
  machine and review timer.
- [`assets/templates`](assets/templates) defines the HTML shape.
- [`internal/ui/immediate_mode_live_test.go`](internal/ui/immediate_mode_live_test.go)
  exercises multi-browser reconciliation through real JaWS WebSockets.

## Verify it

Run both test modes from the module root:

```sh
go test -race ./...
go test ./...
```

The race-enabled run checks concurrent state and multi-client updates. The
plain run also compiles and exercises JaWS's release tag implementation; JaWS
uses a different, debug-friendly implementation under `-race`.

## License

Pretend You're Xyzzy is distributed under the [MIT License](LICENSE).

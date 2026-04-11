# JaWS Notes For Future Work

Read this before changing the JaWS-driven UI in this repo.

## Core mindset

JaWS is not asking for an MVC layer.

- Prefer direct binding to the real model data and the real mutex that protects it.
- Do not invent page/view-model/read-model wrappers unless JaWS itself needs one.
- In this repo, the important domain objects are `*game.Player`, `*game.Room`, `*deck.Deck`, `*deck.WhiteCard`, `*game.Submission`, and `*jaws.Session`.

The earlier mistakes here came from trying to wrap JaWS into a more familiar framework shape. That made the code larger, less direct, and less correct.

## Sessions and players

Use JaWS sessions as player lifetime.

- Store `*game.Player` directly in the JaWS session.
- Treat one JaWS session as one player.
- Let session expiry drive periodic cleanup of disconnected players.
- Keep the nickname cookie only as a seed when a new player object has to be created after session expiry.

Do not bring back `player_id`, `room_code`, or other redundant session keys.

## Binding rules

Bind UI elements directly to real state.

- If a value lives on `*game.Room`, its binder should usually also live on `*game.Room`.
- Use the actual room lock for room-owned state: `bind.New(&r.mu, &r.someField)`.
- Do not use fake mutexes or temporary locals just to get a binder-shaped object.

Examples:

- Score target slider should bind directly to `&room.targetScore`.
- Deck toggles should tag with a comparable value like `(room pointer + deck pointer)`.
- Hand-card and submission actions should tag with semantic comparable values using the actual player/room/card/submission pointers.

## Buttons and click handlers

Do not create dummy binders just to catch clicks.

- Hardcode simple button text in templates.
- Pass the click handler as a separate template parameter.
- Only use binders when there is real bound state.

## Getter purity

Treat getters as pure reads.

- Do not mutate attributes, classes, or other UI state inside `JawsGet`, `JawsGetLocked`, or `JawsGetHTML`.
- Do not call `SetAttr`, `RemoveAttr`, `SetClass`, `RemoveClass`, or `Dirty` from getters.
- Do not hide validation or side effects in getter paths.

If an element needs to start disabled, hidden, or selected:

- Compute that in a helper that returns `template.HTMLAttr`.
- Use that helper in the template at initial render time.

## Error handling

JaWS handlers should return errors.

- Do not manually call `Request.Alert(err)` in event handlers when a plain returned error will do.
- JaWS already turns returned handler errors into alerts.
- The same rule applies to setter hooks used by bound inputs.

Use manual `Request.Alert(...)` only when you truly need request-specific alert behavior outside the normal handler error flow.

## Dirtying

Be precise about what you dirty.

- Dirty only the tags whose rendered output actually depends on the changed state.
- Do not dirty unrelated tags as a convenience shortcut.
- JaWS already discards `nil` tags in `Jaws.Dirty(...)`; do not wrap it just to nil-filter.
- Prefer `Request.Dirty(...)` when already inside a request/element path.
- Use `Jaws.Dirty(...)` for broader app-level broadcasts like room creation/removal or session cleanup.

Example:

- Score target changes should rely on the direct target-score binding and should not dirty unrelated player state.

## Templates over manual HTML

Keep HTML structure in templates, not in Go string concatenation.

- Do not hand-build card or submission markup in Go.
- Use JaWS-aware template-backed HTML getters when button bodies or inner HTML need structured markup.
- Keep partials in their own files.
- Remove redundant `{{define ...}}` wrappers when the file itself is the template body.

Keep text sanitization helpers if needed, but keep DOM structure in templates.

## Template rendering helpers

If you need a custom `bind.HTMLGetter` that renders a partial:

- Render the named template with the dot shape that template actually expects.
- Do not wrap dot in `ui.With` unless the template needs JaWS helper fields like `$.Button`, `$.Container`, `$.Initial`, etc.
- For simple partials like a white-card body, pass the raw view struct directly.

## Container identity and rerendering

JaWS container updates reuse child elements by UI identity.

This matters a lot.

- `ContainerHelper.UpdateContainer()` pools existing child elements by `elem.Ui()`.
- If the same child UI value appears again, JaWS may reuse the old child element instead of rerendering it.
- A tiny wrapper type can be necessary when you intentionally want the container to treat each child as a fresh renderable instance.

In this repo, `templateFrame` in `internal/ui/content.go` currently exists for exactly that reason. Removing it caused live-update tests to fail because child templates stopped being rerendered as expected.

Do not delete that wrapper again unless you also redesign how those sections update.

## Comparable tags

JaWS tags must stay comparable and meaningful.

- Pointers are good tags.
- Small structs containing pointers are good tags.
- Slices, maps, and other non-comparable values are not good tags.

Prefer tags that describe the semantic UI target, not incidental wrapper objects.

## Tests

When testing JaWS bindings:

- Start the JaWS server in harnesses that rely on request/update behavior.
- Use real JaWS elements when calling `JawsSet`, `JawsClick`, or `JawsGetHTML`.
- Keep live-update tests around when changing container identity, dirtying, or tag behavior.

If a cleanup "should not matter" but breaks live updates, inspect JaWS element identity and tag reuse before assuming the tests are wrong.

## Anti-patterns to avoid

Do not reintroduce these:

- Page/view-model layers around direct room/player state.
- Fake binders with fake mutexes and throwaway locals.
- Persisted alert strings for normal handler failures.
- UI mutations inside getters.
- Manually concatenated HTML for cards/submissions.
- Thin wrappers around `Jaws.Dirty(...)` that add no real behavior.
- Session indirection that duplicates what JaWS sessions already provide.

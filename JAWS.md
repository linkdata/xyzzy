# JaWS Notes For Future Work

Read this before changing JaWS-driven UI code.

## Core mindset

JaWS does not require an MVC layer.

- Keep data flow direct between domain state and UI bindings.
- Add abstraction only when it clearly improves correctness, reuse, or readability.
- Prefer small composable helpers over deep wrapper hierarchies.

## Sessions and state ownership

- Keep session data minimal and authoritative.
- Avoid duplicating the same state in multiple session keys.
- Let server-side state ownership remain clear: each mutable value should have one source of truth.

## Binding rules

- Bind to real state and the lock that protects it.
- Use binders for values that are truly stateful and synchronized.
- If a value is computed and has no addressable field, a local/computed binder is acceptable when scoped tightly and tagged meaningfully.

## Buttons and click handlers

- For simple buttons, prefer `ui.Clickable()`.
- Use plain click handlers for click-only interactions.
- Use binders only when the element has real bound value state (not just clicks).
- Keep button label/content concerns separate from click behavior.

## Getter purity

Treat getters as pure reads.

- Do not mutate UI state from `JawsGet*` paths.
- Do not trigger side effects (`Dirty`, alerting, writes) from getters.
- Compute initial attrs/classes in dedicated helpers and render them declaratively.

## Error handling

- Return errors from handlers/setter hooks and let JaWS surface them.
- Use manual request alerts only when custom alert behavior is actually needed.

## Dirtying

- Dirty only tags whose rendered output depends on changed state.
- Avoid broad dirtying as a shortcut.
- Prefer request-scoped dirtying in element/request flows.
- Use app-level dirtying for cross-request broadcasts.

## Templates and partials

- Keep HTML structure in templates, not string concatenation in Go.
- Keep partials small and focused.
- Render pre-sanitized safe HTML fields directly when available.

## Prefer direct `$.Template(...)` rendering

- Prefer `$.Template("...", dot, ...)` over `bind.HTMLGetter` wrappers for rendering card/content bodies.
- Keep the root template dot comparable when it is used as identity/tag (pointer-only structs are ideal).
- For list rendering, range directly over view slices (`HandCardViews`, `SubmissionViews`) instead of ranging domain objects and constructing views inline per row.
- Put computed child lists on view methods (`Cards()`) rather than non-comparable slice fields on the root dot.

## Clickable template wrappers

- For clickable template content, render a wrapper template (not `$.Button` with HTML getter content).
- Wrapper templates must include `id="{{$.Jid}}" {{$.Attrs}}` so JaWS can attach identity, attrs, classes, handlers, and dirty updates correctly.
- Keep interaction semantics explicit on wrapper markup (for example `role="button"` and `tabindex="0"` where keyboard/click behavior is expected).
- Prefer view-dot click handling (`JawsClick`) for template items instead of passing redundant explicit click handlers in `$.Template(...)`.
- Pass explicit click handlers to `$.Template(...)` only when the dot cannot own that behavior.

## Container identity and rerendering

JaWS container updates reuse child elements by identity.

- Be explicit about child UI identity.
- If updates stop rerendering as expected, inspect identity reuse first.
- Wrapper types are valid when you need distinct UI identities.

## Comparable tags

Tags should be comparable and semantically meaningful.

- Prefer pointers or small structs of comparable fields.
- Avoid slices/maps/non-comparable tag values.
- Attach semantic tags where binders/getters are built.

## Nil checks and invariants

- Keep nil checks at real boundaries (request parsing, optional external input, integration seams).
- Avoid defensive nil checks in internal flows where invariants already guarantee non-nil values.
- Prefer invariant-driven code paths over silent fallback behavior for impossible states.

## Tests

- Use real JaWS requests/elements for binder/click/getter behavior tests.
- Keep live-update tests for identity/tag/dirty behavior.
- When a cleanup breaks updates, verify identity and tag semantics before changing tests.

## Anti-patterns to avoid

- Fake binders created only to fit an API shape.
- UI mutations hidden in getters.
- Broad `Dirty(...)` calls that mask dependency mistakes.
- Template helper sprawl when direct method calls or focused helpers are cleaner.

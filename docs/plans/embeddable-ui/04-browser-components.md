---
title: "04 - Host-page browser components"
dependencies: ["03-http-embedding.md"]
effort: "L"
complexity: "high"
---

## Status

Implemented and verified on 2026-09-15.

- `ui/internal/assets/static/js/ui.js` exports `mountManager(root, options)` and
  `mountDisplay(root, options)`, returning `{ destroy }`. `runtime.js` owns each
  mount: root validation, duplicate rejection (`data-ais-mounted`), an optional
  per-instance `options.fetch`, abortable requests, timers, listeners, cleanups,
  and restoring the server-rendered markup on idempotent `destroy()`. The manager
  owns the station editor. Generation and run guards are unchanged and also
  invalidated by destroy.
- Component element IDs derive from `ComponentConfig.ID`; scripts use root-local
  `data-ref` lookups. `css/ui.css` scopes every rule below `.ais-manager` or
  `.ais-display`, exposes `--ais-*` theme and map sizing properties, and uses a
  container query for the display layout. Standalone layout moved to
  `css/page.css`; `app.css` is removed.
- Leaflet 1.9.4 (`leaflet-src.esm.js`, `leaflet.css`, images, BSD LICENSE) is
  bundled under `static/leaflet/`, unmodified and hash-pinned by a Go test. The
  display imports it on mount, never touches `window.L`, keeps tables on import
  failure, and follows root resizes through a `ResizeObserver`, including hidden
  roots. `ui.Config.Tiles` configures the tile URL and plain-text attribution.
- Go API: `UI.StylesheetURL` and `UI.ModuleURL` replace `ManagerResources`,
  `DisplayResources`, `Resource`, and `Resources`. Standalone pages link the same
  stylesheet and load `js/standalone.js`, which mounts every root; pages have no
  inline scripts and only the manager page with a status link loads htmx.
- `ui/testdata/host` and `host-browser.mjs` add a hostile host-page browser check;
  `display-browser.mjs` uses the new markup.

Verification: `task all` passed (714 tests, vet, fmt, deadcode, arch-lint, lint);
`go test -race` passed for `ui` and `display`. Go tests cover unique and resolvable component
IDs across four components, no inline scripts or handlers, tile validation and
escaping, CSS scoping, asset types, and pinned Leaflet hashes. Both browser checks
passed in headless Edge against the combined app and the host fixture, including
destroy during held requests, remount, conflict drafts, custom fetch headers under
host CSRF middleware, a strict Content-Security-Policy, and Leaflet/tile failures.

## Implementation

- Convert manager, station editor, and display initialization to exported module
  mount functions scoped to a supplied root. Manager owns its station editor.
- Resolve API requests from configuration. Add a per-instance fetch option for host
  credentials/CSRF integration without introducing framework dependencies.
- Return an idempotent `destroy()` that clears timers, aborts in-flight reads and
  writes, removes listeners, and disposes Leaflet resources. Aborted writes may
  already have committed server-side; remount always refreshes authoritative state.
- Reject duplicate mounting on a live root with a clear error; permit remount after
  destroy. Preserve existing generation/run guards against late responses.
- Replace global IDs/selectors with root-local lookup and unique accessible IDs.
  Scope styles and expose sizing/theme variables. Keep host layout untouched.
- Package isolated Leaflet assets with license/attribution and configurable tiles.
  Handle resized/initially hidden host containers and dispose resize observers.
- Make standalone pages load and mount exactly these modules. Keep htmx behavior
  confined to standalone pages that use it.

## Verification and acceptance

- Extend browser fixtures to mount manager and display, plus duplicate instances,
  in a host page with unrelated forms, tables, navigation, and colliding old IDs.
- Verify independent requests/state, labels, keyboard focus, host styling, map
  resize, configured paths, and custom fetch headers.
- Deterministically destroy during requests and inspect timers/listeners to confirm
  no later polling, rendering, or retained maps; remount without duplicate work.
- Retain conflict drafts, restart resets, stale views, history bounds, selection
  races, exact NMEA copying, and table fallback when map dependencies fail.
- Components work without host-global Leaflet/htmx or inline executable scripts.

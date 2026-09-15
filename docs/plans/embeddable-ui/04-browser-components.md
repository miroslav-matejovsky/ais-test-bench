---
title: "04 - Host-page browser components"
dependencies: ["03-http-embedding.md"]
effort: "L"
complexity: "high"
---

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

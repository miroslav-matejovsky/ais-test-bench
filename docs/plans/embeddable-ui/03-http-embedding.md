---
title: "03 - Prefix-safe HTTP handlers and rendering"
dependencies: ["01-public-runtime.md", "02-logging.md"]
effort: "L"
complexity: "high"
---

## Status

Implemented and verified on 2026-09-15.

- `internal/ui` is now public `ui`; templates and assets moved to
  `ui/internal/assets`. `ui.New(Config)` validates explicit public URLs:
  `ManagerAPIBase`, `DisplayAPIBase`, `AssetsBase` (absolute, ending in `/`) and
  optional `HomeURL`, `ManagerURL`, `DisplayURL`, `StatusURL` links (absolute path
  or http(s) URL). `internal/urlpath` holds the one canonical base path rule.
- Component templates (`components/`) contain only their root with an inert
  `data-api-base`; `document.tmpl` owns the shell and navigation.
  `RenderManager`/`RenderDisplay`, `ManagerResources`/`DisplayResources`, `Assets`,
  and GET/HEAD page handlers are public. Standalone pages embed the same rendered
  components. Scripts build every request from `data-api-base`.
- `Simulator.API()` and `display.NewHandler(display.Config)` use local routes and
  are mounted with `http.StripPrefix`. Station creation returns a relative
  `Location`. Standalone handlers are `simulator.NewStandaloneHandler` and
  `display.NewStandaloneHandler`; assets moved from `/static/` to `/assets/`, and
  root redirects keep the query.
- `display.HTTPConfig.APIBase` replaces `Origin` and accepts a path prefix.
  `Client.Origin` is removed; the display manager link is explicit
  (`StandaloneConfig.ManagerURL`). `cmd/display` derives it from `-simulator-url`,
  which now accepts a prefix.
- Deferred by plan: document-wide element IDs inside components (one manager and
  one display per page), module mounting, CSS scoping, and bundled Leaflet (step
  04); prefix configuration for standalone serving (step 05).

Verification: `task all` passed (696 tests, vet, fmt, deadcode, arch-lint, lint).
`go test -race` passed for `ui`, `display`, `simulator`, `internal/app`, and
`internal/urlpath`. `simulator/embedding_test.go` covers root, nested, independent,
separately mounted, and prefix-stripping proxy layouts, host routes, middleware,
and an authenticated prefixed remote source. The optional browser fixture
`ui/testdata/display-browser.mjs` was not run in this step.

## Implementation

- Publish `ui`, retaining embedded templates/assets and private rendering helpers.
  Separate host-page component templates from standalone document and navigation.
- Implement explicit URL configuration, validation, component IDs, asset URLs,
  fragment renderers, and full-page handlers from the README contract.
- Convert low-level API routes to local paths. Define mount/strip ownership once
  per public API. Ensure redirects, status fragments, JSON links, scripts, CSS,
  and navigation retain configured prefixes.
- Extend the remote source to API base URLs with paths and an optional HTTP client.
  Preserve deadlines/limits and define cleanup of owned transports.
- Escape configuration via templates and inert data attributes. Reject unsafe
  URL schemes. Expose middleware-compatible handlers without modifying a global mux.

## Verification and acceptance

- Table-driven `httptest` coverage for root, nested paths, independent instances,
  separately mounted assets/APIs, and a proxy that strips the external prefix.
- Verify method handling, redirects, query preservation, static content types,
  404 behavior, and no accidental interception of the host's root or `/api` routes.
- Exercise remote prefix joining, client authentication, cancellation, timeouts,
  and ownership. API-only local reads require no public HTTP server.
- Component HTML has no document shell and escapes hostile labels/configuration.
- Middleware can protect manager writes and both APIs. No library CORS or auth
  policy overrides the host's policy.

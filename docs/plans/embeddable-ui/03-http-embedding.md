---
title: "03 - Prefix-safe HTTP handlers and rendering"
dependencies: ["01-public-runtime.md", "02-logging.md"]
effort: "L"
complexity: "high"
---

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

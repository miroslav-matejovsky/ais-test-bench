# Frontends

`admin` and `viewer` are independent frontend bundles with separate asset roots.
Each Go package embeds its own `assets/` directory using `embed.FS`.
The current HTML files identify scaffold status.

Use two independent React builds as the initial direction. Each build writes its
distribution into its own `assets/` directory before Go compilation. Serve assets
from `fs.Sub(Files, "assets")` under `/admin/` or `/viewer/`. Configure each build's
base URL to match its mount. Same-origin JSON requests use `/api/v1`.
Each UI can be enabled separately. Neither imports the other's application state.

Admin owns scenario CRUD, target CRUD, simulation controls, and system status.
Viewer owns chart display, vessel markers, AIS labels, selection, and playback
controls. It polls consistent snapshots; simulation remains server-owned.
Start with hash routing to keep embedded file serving simple.

Bundle scripts, styles, fonts, and required chart assets for local use. Runtime
requires only the executable plus configuration and mutable data. Node is a build
tool, not a second application process.

HTMX + Templ remains an alternative. Templ components compile into Go; static
assets still use embed.FS. HTMX requests expect HTML fragments, so add frontend
presentation handlers using the same use cases while retaining JSON REST endpoints.
Do not send HTML fragments from the versioned JSON API.

Sources: [React](https://react.dev/learn),
[Templ](https://templ.guide/), [HTMX](https://htmx.org/docs/).

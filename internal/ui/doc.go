// Package ui renders the Manager, Display, Home, and Status pages and serves
// embedded static files. It holds no simulation or display state: page scripts
// poll same-origin JSON APIs once per real second, also while the simulation is
// paused.
//
// Manager reads the simulator fleet, history, and metadata (/api/*) on every
// poll and renders them only when all three carry the same simulation ID. It
// changes the vessel count and simulation speed. A request generation counter
// discards polls that overlap a write, so an older poll never undoes a confirmed
// change, and a button stays disabled while its write is pending. Polls never
// overwrite a field the user edited or focused. A new simulation ID resets the
// per-run form state before the new run is rendered. Display uses the display
// API (/display/api/vessels), Leaflet, and OpenStreetMap.
//
// Both pages show the virtual UTC simulation time and effective speed from the
// last successful response, without local extrapolation, and show connection
// freshness separately as the real local receipt time. After a failed poll they
// keep the last view and mark the clock, and on Display the map, as stale.
//
// Components compose the pages they serve. NewPages takes the header links for
// exactly those pages, so a standalone component never advertises a page it
// does not serve. Static is mounted once per listener at /static/.
//
// Templates and static files come from package assets. The shared template set
// holds the "base" layout, which renders the links through the "nav" template
// function. Each page file defines "page:title" and "page:content", plus any
// fragments used only by that page. A handler renders "base" for a full page,
// or a single fragment when htmx asks for a partial.
//
// htmx 4 sends HX-Request-Type: partial for swaps into a target element and
// HX-Request-Type: full for body swaps and history restores. Only "partial"
// selects a fragment. Responses set Vary: HX-Request-Type so caches keep both
// variants apart.
package ui

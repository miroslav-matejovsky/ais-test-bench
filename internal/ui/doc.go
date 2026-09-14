// Package ui renders the Manager, Display, Home, and Status pages and serves
// embedded static files. It holds no simulation or display state: page scripts
// poll same-origin JSON APIs once per second. Manager uses the simulator API
// (/api/*) to adjust the vessel count and show recent AIS sentences. Display
// uses the display API (/display/api/vessels), Leaflet, and OpenStreetMap.
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

// Package ui serves the server-rendered HTML user interfaces, Manager and Display.
//
// Templates and static files come from package assets. The shared template set
// holds the "base" layout. Each page file defines "page:title" and
// "page:content", plus any fragments used only by that page. A handler renders
// "base" for a full page, or a single fragment when htmx asks for a partial.
//
// htmx 4 sends HX-Request-Type: partial for swaps into a target element and
// HX-Request-Type: full for body swaps and history restores. Only "partial"
// selects a fragment. Responses set Vary: HX-Request-Type so caches keep both
// variants apart.
package ui

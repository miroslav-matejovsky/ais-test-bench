// Package ui serves the Manager and Display pages and a shared simulation API.
// GET /api/vessels returns live navigation snapshots; PUT /api/vessels accepts
// a JSON count from 0 through 100. GET /api/messages returns retained reports.
// Browser scripts poll once per second. Display uses Leaflet and OpenStreetMap;
// Manager adjusts vessel count and shows recent AIS sentences.
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

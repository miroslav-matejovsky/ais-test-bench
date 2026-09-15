// Package ui renders embeddable manager and display components, standalone
// pages, and embedded assets. It holds no simulation or display state: component
// scripts poll their configured JSON APIs once per real second, also while the
// simulation is paused.
//
// # URLs and mounting
//
// New validates Config before anything is served and changes no globals. Every
// URL is a public browser URL, including an external prefix removed by a reverse
// proxy; nothing is derived from request paths or forwarded headers.
// ManagerAPIBase, DisplayAPIBase, and AssetsBase are absolute base paths ending
// in "/", containing only letters, digits, "-._~" and separators, without empty
// or dot segments. An empty API base disables that component. HomeURL,
// ManagerURL, DisplayURL, and StatusURL are optional links: absolute paths or
// absolute http(s) URLs without user info. Relative, scheme-relative,
// javascript:, data:, and other links are rejected.
//
// Assets serves local paths such as /css/app.css; mount it at AssetsBase with
// http.StripPrefix. Page handlers ignore their mount path, so mount them wherever
// the configured links point. Pages and assets answer GET and HEAD and return 405
// with Allow otherwise. No handler sets CORS or authentication policy, so host
// middleware can wrap each one, and the package never registers routes on a mux.
//
// # Components and pages
//
// RenderManager and RenderDisplay write one root element with the caller's ID,
// class ais-manager or ais-display, a data-ais-manager or data-ais-display marker,
// and the escaped API base in the inert data-api-base attribute. They write no
// document shell, navigation, headings, or scripts, and write nothing on error.
// Hosts load ManagerResources or DisplayResources once. Scripts locate their root
// by its marker and build every request below data-api-base. Element IDs inside
// components are still document-wide, so one page holds at most one manager and
// one display.
//
// ManagerPage, DisplayPage, HomePage, and StatusPage wrap the same components in
// a document with header links from Config. The manager page loads htmx and adds
// a status check only when StatusURL is set. Pages render into a buffer; a
// failure is logged with component=ui and returns 500 without partial HTML.
// Templates come from the internal assets package: document.tmpl, components/,
// and pages/, parsed once in New. Each page file defines "page:title" and
// "page:content", plus fragments used only by that page.
//
// # Browser behavior
//
// Manager reads vessels, messages, and metadata below its API base on every poll
// and renders them only when all three carry the same simulation ID. It changes
// the vessel count and simulation speed. A request generation counter discards
// polls that overlap a write, so an older poll never undoes a confirmed change,
// and a button stays disabled while its write is pending. Polls never overwrite a
// field the user edited or focused. A new simulation ID resets the per-run form
// state before the new run is rendered.
//
// Display polls observations below its API base with all, one, or several station
// IDs. It renders simulator-supplied channel A/B coverage, station capabilities
// and counters, received AIS targets, receiver provenance, and successful
// messages. Leaflet markers use received positions, mute stale targets, and
// optionally show lost targets. Geometry is cached by station RF revision;
// ordinary polls retain map zoom, selected details, keyed table controls, focus,
// and scroll. Tables keep working when Leaflet or tiles fail. Fresh/stale splits
// and last seen derive from selected retained observations; unselected sites show
// current/lost totals.
//
// The display inspector polls a station's stations/{id}/receptions history
// independently. It retains at most 200 rows and one explicitly pinned message,
// uses exact decimal-string cursors, shows gaps, and can pause while main
// snapshots continue. Message details and clipboard copies retain original NMEA
// bytes and reception-time receiver settings. Selection generations and run
// identities reject late replies; 404 resets removed station selection and 409
// clears history cursors. A new run clears all received and selected state.
// Browser-only deterministic checks are documented in testdata/README.md.
//
// The manager's stations.js independently polls stations below its API base and
// renders site configuration and receiving capabilities. It creates, edits,
// disables, and deletes stations through revision-checked simulator routes,
// including a preset disabling channel B. Drafts retain their starting
// run/revision across polls. Conflicts display current values beside the
// preserved draft and require explicit reconciliation. A draft from a removed
// site or previous run can be copied into a new site, never silently applied to a
// reused run-local ID. Inputs are disabled during writes, errors appear beside
// fields, and all labels use text-only DOM APIs.
//
// Both components show the virtual UTC simulation time and effective speed from
// the last successful response, without local extrapolation, and show connection
// freshness separately as the real local receipt time. After a failed poll they
// keep the last view and mark the clock, and on Display the entire view, as stale.
//
// htmx 4 sends HX-Request-Type: partial for swaps into a target element and
// HX-Request-Type: full for body swaps and history restores. Only "partial"
// selects the status fragment. Page responses set Vary: HX-Request-Type so caches
// keep both variants apart.
package ui

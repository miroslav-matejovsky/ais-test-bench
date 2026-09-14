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
// per-run form state before the new run is rendered.
//
// Display polls /display/api/observations with all, one, or several station IDs.
// It renders simulator-supplied channel A/B coverage, station capabilities and
// counters, received AIS targets, receiver provenance, and successful messages.
// Leaflet markers use received positions, mute stale targets, and optionally show
// lost targets. Geometry is cached by station RF revision; ordinary polls retain
// map zoom, selected details, keyed table controls, focus, and scroll. Tables keep
// working when Leaflet or tiles fail. Fresh/stale splits and last seen derive
// from selected retained observations; unselected sites show current/lost totals.
//
// The display inspector polls a station's /display/api/stations/{id}/receptions
// history independently. It retains at most 200 rows and one explicitly pinned
// message, uses exact decimal-string cursors, shows gaps, and can pause while
// main snapshots continue. Message details and clipboard copies retain original
// NMEA bytes and reception-time receiver settings. Selection generations and
// run identities reject late replies; 404 resets removed station selection and
// 409 clears history cursors. A new run clears all received and selected state.
// Browser-only deterministic checks are documented in testdata/README.md.
//
// The manager's stations.js independently polls /api/stations and renders site
// configuration and receiving capabilities. It creates, edits, disables, and
// deletes stations through revision-checked simulator routes, including a preset
// disabling channel B. Drafts retain their starting run/revision across polls.
// Conflicts display current values beside the preserved draft and require explicit
// reconciliation. A draft from a removed site or previous run can be copied into
// a new site, never silently applied to a reused run-local ID. Inputs are disabled
// during writes, errors appear beside fields, and all labels use text-only DOM APIs.
//
// Both pages show the virtual UTC simulation time and effective speed from the
// last successful response, without local extrapolation, and show connection
// freshness separately as the real local receipt time. After a failed poll they
// keep the last view and mark the clock, and on Display the entire view, as stale.
//
// Components compose the pages they serve. NewPages takes the header links for
// local pages. The standalone display also links to the configured simulator's
// external manager. Static is mounted once per listener at /static/.
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

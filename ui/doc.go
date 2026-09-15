// Package ui renders embeddable manager and display components, standalone
// pages, and embedded assets. It holds no simulation or display state: mounted
// components poll their configured JSON APIs once per real second, also while
// the simulation is paused.
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
// javascript:, data:, and other links are rejected. Tiles optionally replaces the
// OpenStreetMap tile source with a URL template containing {z}, {x}, and {y} and
// a required plain-text attribution.
//
// Assets serves local paths such as /css/ui.css and /js/ui.js; mount it at
// AssetsBase with http.StripPrefix. Page handlers ignore their mount path, so
// mount them wherever the configured links point. Pages and assets answer GET and
// HEAD and return 405 with Allow otherwise. No handler sets CORS or
// authentication policy, so host middleware can wrap each one, and the package
// never registers routes on a mux.
//
// # Components and pages
//
// RenderManager and RenderDisplay write one root element with the caller's ID,
// class ais-manager or ais-display, a data-ais-manager or data-ais-display marker,
// and the escaped API base in the inert data-api-base attribute. The display root
// also carries its tile source in data-tile-* attributes. They write no document
// shell, navigation, headings, scripts, or inline event handlers, and write
// nothing on error. Element IDs inside a component start with its ID and "-", so
// one page holds any number of components with distinct IDs. Scripts find their
// elements by data-ref inside their root.
//
// A host page links StylesheetURL once and mounts rendered roots from its own
// module script with the module at ModuleURL:
//
//	import { mountManager, mountDisplay } from "/tools/ais/assets/js/ui.js";
//	const manager = mountManager(document.getElementById("fleet"), { fetch: hostFetch });
//	manager.destroy();
//
// ManagerPage, DisplayPage, HomePage, and StatusPage wrap the same components in
// a document with header links from Config. Component pages link the component
// stylesheet and page.css and load standalone.js, a module that mounts every
// component root with the same functions; pages contain no inline scripts. The
// manager page loads htmx and adds a status check only when StatusURL is set.
// Pages render into a buffer; a failure is logged with component=ui and returns
// 500 without partial HTML. Templates come from the internal assets package:
// document.tmpl, components/, and pages/, parsed once in New. Each page file
// defines "page:title" and "page:content", plus fragments used only by that page.
//
// # Browser lifecycle
//
// Rendering starts no browser work. mountManager(root, options) and
// mountDisplay(root, options) start it and return { destroy }. options.fetch
// optionally replaces the browser fetch for that instance, for example to add
// authentication or CSRF headers; it receives each request URL and a RequestInit
// carrying an abort signal, and returns a Response promise. Mounting a live root
// again, or a root of the other component, throws. The manager owns its station
// editor.
//
// destroy is idempotent. It aborts in-flight reads and writes, clears timers,
// disconnects the map resize observer, removes the Leaflet map, and restores the
// server-rendered markup, which drops every listener and dynamic node. A write
// aborted by destroy may already be committed by the server; a later mount of the
// same root reads the current state. Generation and run guards discard replies
// that arrive after destroy or after a newer request.
//
// The stylesheet scopes every rule below .ais-manager or .ais-display and imports
// the bundled Leaflet stylesheet, whose classes use the leaflet- prefix. Hosts
// theme components with optional custom properties on a root or an ancestor:
// --ais-accent, --ais-border, --ais-surface, --ais-error, --ais-warning,
// --ais-map-height, and --ais-map-min-height. The display layout switches to one
// column through a container query on its root, so the root needs a definite
// inline size from the host layout.
//
// The display imports the bundled Leaflet 1.9.4 ES module when mounted, so a host
// window.L is neither required nor changed, and a failed import leaves the tables
// working. The map follows root resizes, including a root that starts hidden.
// Components need no inline scripts, external scripts, or htmx. A host
// Content-Security-Policy must allow the asset origin in script-src and style-src,
// the API origins in connect-src, and the tile origin in img-src. Leaflet sets
// element styles through the CSSOM, which style-src 'self' permits.
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
// Browser-only checks are documented in testdata/README.md.
//
// The manager's station editor independently polls stations below its API base
// and renders site configuration and receiving capabilities. It creates, edits,
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

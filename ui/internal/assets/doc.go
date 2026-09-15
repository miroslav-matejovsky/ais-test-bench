// Package assets embeds the HTML templates and static files used by package ui.
//
// HTMLFiles is rooted at html/: document.tmpl is the standalone page shell with
// header navigation, components/ holds the manager and display component markup
// rendered into host pages, and pages/ holds one file per standalone page that
// defines "page:title" and "page:content". Component element IDs derive from the
// component ID; scripts address elements by data-ref.
//
// StaticFiles is rooted at static/:
//
//   - css/ui.css: component styles scoped below .ais-manager and .ais-display;
//     imports leaflet/leaflet.css.
//   - css/page.css: standalone document layout.
//   - js/ui.js: public ES module exporting mountManager and mountDisplay.
//   - js/runtime.js: the mount lifecycle shared by components: root validation,
//     abortable fetch, timers, listeners, and destroy.
//   - js/manager.js and js/stations.js: fleet and speed controls, and the station
//     table and editor owned by the manager.
//   - js/display.js: the observation display; imports Leaflet when mounted.
//   - js/standalone.js: mounts every component root of a standalone page.
//   - js/htmx.min.js: vendored htmx, loaded only by standalone status checks.
//   - leaflet/: the unmodified leaflet-src.esm.js, leaflet.css, and images/ from
//     the Leaflet 1.9.4 npm dist directory, with its BSD 2-Clause LICENSE.
//
// Both are sub-filesystems, so callers never use the html/ or static/ prefix.
// Modules import each other by relative URL. To update Leaflet, replace leaflet/
// with a release's files and update the pinned hashes in ui's
// TestBundledLeafletIsPinned. OpenStreetMap supplies map tiles by default.
package assets

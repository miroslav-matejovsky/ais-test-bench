// Package assets embeds the HTML templates and static files used by package ui.
//
// HTMLFiles is rooted at html/: document.tmpl is the standalone page shell with
// header navigation, components/ holds the manager and display component markup
// rendered into host pages, and pages/ holds one file per standalone page that
// defines "page:title" and "page:content". StaticFiles is rooted at static/ and
// holds CSS, the vendored htmx build used only by standalone status checks, and
// the live UI scripts. Leaflet is loaded from a CDN; OpenStreetMap supplies map
// tiles. Both are sub-filesystems, so callers never use the html/ or static/
// prefix.
//
// Scripts read their API base from the data-api-base attribute of their
// component root. The manager loads manager.js for fleet/time and stations.js for
// station tables, revision-aware configuration forms, shadow sectors, and channel
// capability presets.
package assets

// Package assets embeds the HTML templates and static files served by package ui.
//
// HTMLFiles is rooted at html/: base.tmpl holds the shared layout and pages/
// holds one file per page. StaticFiles is rooted at static/ and holds CSS and
// the vendored htmx build. Both are sub-filesystems, so callers never use the
// html/ or static/ prefix.
package assets

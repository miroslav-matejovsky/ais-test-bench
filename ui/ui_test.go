package ui_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/ui"
)

// nestedConfig configures every URL below the /tools/ais public prefix.
func nestedConfig() ui.Config {
	return ui.Config{
		ManagerAPIBase: "/tools/ais/api/", DisplayAPIBase: "/tools/ais/display/api/", AssetsBase: "/tools/ais/assets/",
		HomeURL: "/tools/ais/", ManagerURL: "/tools/ais/manager", DisplayURL: "/tools/ais/display", StatusURL: "/tools/ais/status",
		Logger: slog.New(slog.DiscardHandler),
	}
}

func newUI(t *testing.T, config ui.Config) *ui.UI {
	t.Helper()
	u, err := ui.New(config)
	require.NoError(t, err)
	return u
}

func pageHandler(t *testing.T, handler http.Handler, err error) http.Handler {
	t.Helper()
	require.NoError(t, err)
	return handler
}

func request(handler http.Handler, method, target string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for key, value := range header {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestNewValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*ui.Config)
		wantErr string
	}{
		{name: "nested prefix", change: func(*ui.Config) {}},
		{name: "root layout", change: func(c *ui.Config) {
			*c = ui.Config{ManagerAPIBase: "/api/", DisplayAPIBase: "/display/api/", AssetsBase: "/assets/", HomeURL: "/"}
		}},
		{name: "separately mounted", change: func(c *ui.Config) {
			c.ManagerAPIBase, c.DisplayAPIBase, c.AssetsBase = "/backend/simulator/", "/backend/display/", "/"
		}},
		{name: "only display", change: func(c *ui.Config) { c.ManagerAPIBase = "" }},
		{name: "external links", change: func(c *ui.Config) {
			c.ManagerURL, c.DisplayURL = "https://simulator.example/tools/ais/manager", "http://display.example:8081/display?stations=all"
		}},
		{name: "local tiles", change: func(c *ui.Config) {
			c.Tiles = ui.MapTiles{URL: "/tiles/{z}/{x}/{y}.png", Attribution: "Local tiles"}
		}},
		{name: "remote tiles with credit link", change: func(c *ui.Config) {
			c.Tiles = ui.MapTiles{URL: "https://tiles.example/{z}/{x}/{y}.png?key=k", Attribution: "Example", AttributionURL: "https://tiles.example/credits"}
		}},
		{name: "missing assets", change: func(c *ui.Config) { c.AssetsBase = "" }, wantErr: "AssetsBase"},
		{name: "assets without trailing slash", change: func(c *ui.Config) { c.AssetsBase = "/tools/ais/assets" }, wantErr: "AssetsBase"},
		{name: "relative API", change: func(c *ui.Config) { c.ManagerAPIBase = "api/" }, wantErr: "ManagerAPIBase"},
		{name: "dot segment", change: func(c *ui.Config) { c.DisplayAPIBase = "/tools/../display/api/" }, wantErr: "DisplayAPIBase"},
		{name: "encoded separator", change: func(c *ui.Config) { c.DisplayAPIBase = "/tools%2Fais/" }, wantErr: "DisplayAPIBase"},
		{name: "query in base", change: func(c *ui.Config) { c.ManagerAPIBase = "/api/?x=1/" }, wantErr: "ManagerAPIBase"},
		{name: "absolute URL base", change: func(c *ui.Config) { c.ManagerAPIBase = "https://example.test/api/" }, wantErr: "ManagerAPIBase"},
		{name: "javascript link", change: func(c *ui.Config) { c.HomeURL = "javascript:alert(1)" }, wantErr: "HomeURL"},
		{name: "data link", change: func(c *ui.Config) { c.ManagerURL = "data:text/html,<script>alert(1)</script>" }, wantErr: "ManagerURL"},
		{name: "scheme-relative link", change: func(c *ui.Config) { c.DisplayURL = "//evil.example/display" }, wantErr: "DisplayURL"},
		{name: "relative link", change: func(c *ui.Config) { c.DisplayURL = "display" }, wantErr: "DisplayURL"},
		{name: "user info link", change: func(c *ui.Config) { c.StatusURL = "https://user:secret@example.test/status" }, wantErr: "StatusURL"},
		{name: "backslash link", change: func(c *ui.Config) { c.StatusURL = "/\\evil.example" }, wantErr: "StatusURL"},
		{name: "tiles without attribution", change: func(c *ui.Config) { c.Tiles = ui.MapTiles{URL: "/tiles/{z}/{x}/{y}.png"} }, wantErr: "Tiles"},
		{name: "attribution without tiles", change: func(c *ui.Config) { c.Tiles = ui.MapTiles{Attribution: "Credit"} }, wantErr: "Tiles"},
		{name: "tiles without placeholder", change: func(c *ui.Config) { c.Tiles = ui.MapTiles{URL: "/tiles/{z}/{x}.png", Attribution: "Credit"} }, wantErr: "{y}"},
		{name: "javascript tiles", change: func(c *ui.Config) {
			c.Tiles = ui.MapTiles{URL: "javascript:{z}{x}{y}", Attribution: "Credit"}
		}, wantErr: "Tiles.URL: "},
		{name: "unsafe credit link", change: func(c *ui.Config) {
			c.Tiles = ui.MapTiles{URL: "/tiles/{z}/{x}/{y}.png", Attribution: "Credit", AttributionURL: "data:text/html,x"}
		}, wantErr: "AttributionURL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := nestedConfig()
			tt.change(&config)
			_, err := ui.New(config)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// scriptTag matches opening script tags.
var scriptTag = regexp.MustCompile(`<script[^>]*>`)

func TestPagesKeepConfiguredPrefixes(t *testing.T) {
	u := newUI(t, nestedConfig())
	manager, err := u.ManagerPage()
	manager = pageHandler(t, manager, err)
	display, err := u.DisplayPage()
	display = pageHandler(t, display, err)

	tests := []struct {
		name        string
		handler     http.Handler
		header      map[string]string
		contains    []string
		notContains []string
	}{
		{
			name: "manager", handler: manager,
			contains: []string{
				"<title>Manager", "<h1>Manager</h1>", `<a href="/tools/ais/">AIS Test Bench</a>`,
				`<a href="/tools/ais/manager">Manager</a>`, `<a href="/tools/ais/display">Display</a>`,
				`<link rel="stylesheet" href="/tools/ais/assets/css/ui.css">`, `<link rel="stylesheet" href="/tools/ais/assets/css/page.css">`,
				`<script type="module" src="/tools/ais/assets/js/standalone.js"></script>`,
				`<script defer src="/tools/ais/assets/js/htmx.min.js"></script>`, `hx-get="/tools/ais/status"`,
				`<div id="ais-manager" class="ais-manager" data-ais-manager data-api-base="/tools/ais/api/">`,
				`href="/tools/ais/api/stations"`, `href="/tools/ais/api/observations"`, `href="/tools/ais/api/messages"`, `href="/tools/ais/api/metadata"`,
				`<label for="ais-manager-vessel-count">`,
				`<input id="ais-manager-vessel-count" data-ref="vessel-count" name="count" type="number" min="0" step="1" required disabled>`,
				`<input id="ais-manager-speed" data-ref="speed" name="speed" type="number" min="0" max="100" step="0.01" required disabled>`,
				`data-ref="stations-body"`, `data-ref="station-form"`, `data-ref="station-fields" disabled`, `data-ref="station-conflict" hidden`,
			},
			notContains: []string{`"/api/`, `"/static/`, `"/assets/`, `"/status"`, "unpkg", "leaflet"},
		},
		{
			name: "display", handler: display,
			contains: []string{
				"<title>Display", "<h1>Display</h1>",
				`<link rel="stylesheet" href="/tools/ais/assets/css/ui.css">`,
				`<script type="module" src="/tools/ais/assets/js/standalone.js"></script>`,
				`<div id="ais-display" class="ais-display" data-ais-display data-api-base="/tools/ais/display/api/"`,
				`data-tile-template="https://tile.openstreetmap.org/{z}/{x}/{y}.png"`,
				`data-tile-attribution="© OpenStreetMap contributors"`,
				`data-tile-attribution-url="https://www.openstreetmap.org/copyright"`,
				`href="/tools/ais/manager">Open manager</a>`, `data-ref="map"`, `data-ref="station-selection"`,
				`<label for="ais-display-raw-nmea">`, `id="ais-display-raw-nmea" class="ais-raw-nmea" data-ref="raw-nmea" readonly`,
			},
			notContains: []string{`"/display/api/`, `"/api/`, "htmx", "unpkg"},
		},
		{
			name: "home", handler: u.HomePage(),
			contains: []string{
				"<title>Home", `<li><a href="/tools/ais/manager">Manager</a>`, `<li><a href="/tools/ais/display">Display</a>`,
				`<link rel="stylesheet" href="/tools/ais/assets/css/page.css">`,
			},
			notContains: []string{"<script", "ui.css"},
		},
		{name: "status page", handler: u.StatusPage(), contains: []string{"<html", "<h1>Status</h1>", "Uptime:"}},
		{name: "status history restore", handler: u.StatusPage(), header: map[string]string{"HX-Request-Type": "full"}, contains: []string{"<html", "Uptime:"}},
		{name: "status fragment", handler: u.StatusPage(), header: map[string]string{"HX-Request-Type": "partial"}, contains: []string{"Uptime:"}, notContains: []string{"<html", "<h1>"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := request(tt.handler, http.MethodGet, "/anything?ignored=1", tt.header)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
			require.Equal(t, "HX-Request-Type", rec.Header().Get("Vary"))
			for _, s := range tt.contains {
				require.Contains(t, rec.Body.String(), s)
			}
			for _, s := range tt.notContains {
				require.NotContains(t, rec.Body.String(), s)
			}
			for _, tag := range scriptTag.FindAllString(rec.Body.String(), -1) {
				require.Contains(t, tag, ` src="`, "pages have no inline scripts")
			}

			require.Equal(t, http.StatusOK, request(tt.handler, http.MethodHead, "/", tt.header).Code)
			rejected := request(tt.handler, http.MethodPost, "/", tt.header)
			require.Equal(t, http.StatusMethodNotAllowed, rejected.Code)
			require.Equal(t, "GET, HEAD", rejected.Header().Get("Allow"))
		})
	}
}

func TestOptionalLinksAndDisabledComponents(t *testing.T) {
	u := newUI(t, ui.Config{DisplayAPIBase: "/display/api/", AssetsBase: "/assets/"})
	display, err := u.DisplayPage()
	display = pageHandler(t, display, err)
	body := request(display, http.MethodGet, "/", nil).Body.String()
	require.Contains(t, body, "<span>AIS Test Bench</span>")
	require.NotContains(t, body, "Open manager")
	require.NotContains(t, body, "<nav>\n            <a")

	_, err = u.ManagerPage()
	require.ErrorContains(t, err, "API base is not configured")
	var out bytes.Buffer
	require.ErrorContains(t, u.RenderManager(&out, ui.ComponentConfig{ID: "manager"}), "API base is not configured")
	require.Zero(t, out.Len())
}

func TestComponentsRenderOnlyTheirRoot(t *testing.T) {
	u := newUI(t, nestedConfig())
	for name, render := range map[string]func(*bytes.Buffer, ui.ComponentConfig) error{
		"manager": func(w *bytes.Buffer, c ui.ComponentConfig) error { return u.RenderManager(w, c) },
		"display": func(w *bytes.Buffer, c ui.ComponentConfig) error { return u.RenderDisplay(w, c) },
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			require.NoError(t, render(&out, ui.ComponentConfig{ID: "host-" + name + "_1"}))
			html := strings.TrimSpace(out.String())
			require.True(t, strings.HasPrefix(html, `<div id="host-`+name+`_1" class="ais-`+name+`" data-ais-`+name+` data-api-base="/tools/ais/`), html[:min(len(html), 120)])
			require.True(t, strings.HasSuffix(html, "</div>"))
			for _, shell := range []string{"<!doctype", "<html", "<head", "<body", "<header", "<nav", "<main", "<script", "<link", "<style", "<title", "<h1", "hx-get"} {
				require.NotContains(t, strings.ToLower(html), shell)
			}
			require.NotRegexp(t, `\son[a-z]+=`, html, "no inline event handlers")

			for _, id := range []string{"", "1manager", "host manager", `x" onclick="alert(1)`, "café", strings.Repeat("a", 65)} {
				out.Reset()
				require.ErrorContains(t, render(&out, ui.ComponentConfig{ID: id}), "invalid component ID")
				require.Zero(t, out.Len(), "a failed render writes nothing")
			}
		})
	}
}

// TestComponentIDsAreUniqueAndResolve renders two managers and two displays
// into one page, as a host would, and checks that element IDs never collide and
// every label and ARIA reference points into its own component.
func TestComponentIDsAreUniqueAndResolve(t *testing.T) {
	u := newUI(t, nestedConfig())
	var page bytes.Buffer
	roots := []string{"fleet-a", "fleet-b", "map-a", "map-b"}
	require.NoError(t, u.RenderManager(&page, ui.ComponentConfig{ID: roots[0]}))
	require.NoError(t, u.RenderManager(&page, ui.ComponentConfig{ID: roots[1]}))
	require.NoError(t, u.RenderDisplay(&page, ui.ComponentConfig{ID: roots[2]}))
	require.NoError(t, u.RenderDisplay(&page, ui.ComponentConfig{ID: roots[3]}))

	ids := make(map[string]bool)
	for _, match := range regexp.MustCompile(`\sid="([^"]*)"`).FindAllStringSubmatch(page.String(), -1) {
		id := match[1]
		require.False(t, ids[id], "duplicate id %q", id)
		ids[id] = true
		owned := false
		for _, root := range roots {
			owned = owned || id == root || strings.HasPrefix(id, root+"-")
		}
		require.True(t, owned, "id %q is not derived from a component ID", id)
	}
	references := regexp.MustCompile(`\s(?:for|aria-describedby|aria-labelledby)="([^"]*)"`).FindAllStringSubmatch(page.String(), -1)
	require.NotEmpty(t, references)
	for _, match := range references {
		require.True(t, ids[match[1]], "reference %q has no element", match[1])
	}
}

func TestHostileConfigurationIsEscaped(t *testing.T) {
	config := nestedConfig()
	config.ManagerURL = `https://example.test/manager?q="><script>alert(1)</script>`
	config.HomeURL = `/tools/ais/?next='onmouseover='alert(1)`
	config.Tiles = ui.MapTiles{URL: `/tiles/{z}/{x}/{y}.png?s="><img>`, Attribution: `<b>"Local" & tiles</b>`, AttributionURL: "/tiles/credits"}
	u := newUI(t, config)
	display, err := u.DisplayPage()
	display = pageHandler(t, display, err)
	var component bytes.Buffer
	require.NoError(t, u.RenderDisplay(&component, ui.ComponentConfig{ID: "display"}))
	for _, html := range []string{request(display, http.MethodGet, "/", nil).Body.String(), component.String()} {
		require.NotContains(t, html, "<script>alert")
		require.NotContains(t, html, `q="><`)
		require.NotContains(t, html, `'onmouseover=`)
		require.NotContains(t, html, "<img>")
		require.NotContains(t, html, "<b>")
	}
	require.Contains(t, component.String(), `href="https://example.test/manager?q=%22%3e%3cscript%3ealert%281%29%3c/script%3e">Open manager</a>`)
	require.Contains(t, component.String(), `data-tile-template="/tiles/{z}/{x}/{y}.png?s=&#34;&gt;&lt;img&gt;"`)
	require.Contains(t, component.String(), `data-tile-attribution="&lt;b&gt;&#34;Local&#34; &amp; tiles&lt;/b&gt;"`)
	require.Contains(t, component.String(), `data-tile-attribution-url="/tiles/credits"`)
}

func TestAssetURLs(t *testing.T) {
	u := newUI(t, ui.Config{ManagerAPIBase: "/api/", DisplayAPIBase: "/display/api/", AssetsBase: "/ais-assets/"})
	require.Equal(t, "/ais-assets/css/ui.css", u.StylesheetURL())
	require.Equal(t, "/ais-assets/js/ui.js", u.ModuleURL())
}

func TestAssets(t *testing.T) {
	u := newUI(t, nestedConfig())
	mux := http.NewServeMux()
	mux.Handle("/tools/ais/assets/", http.StripPrefix("/tools/ais/assets", u.Assets()))
	const css, js = "text/css; charset=utf-8", "text/javascript; charset=utf-8"
	tests := []struct {
		method, target string
		wantStatus     int
		wantType       string
		contains       string
		notContains    string
	}{
		{method: http.MethodGet, target: "/tools/ais/assets/css/ui.css", wantStatus: http.StatusOK, wantType: css, contains: `@import url("../leaflet/leaflet.css");`},
		{method: http.MethodGet, target: "/tools/ais/assets/css/page.css", wantStatus: http.StatusOK, wantType: css, contains: "body"},
		{method: http.MethodGet, target: "/tools/ais/assets/js/ui.js", wantStatus: http.StatusOK, wantType: js, contains: `export { mountManager } from "./manager.js";`},
		{method: http.MethodGet, target: "/tools/ais/assets/js/manager.js", wantStatus: http.StatusOK, wantType: js, contains: "export function mountManager", notContains: `"/api/`},
		{method: http.MethodGet, target: "/tools/ais/assets/js/stations.js", wantStatus: http.StatusOK, wantType: js, contains: "export function startStations", notContains: "/api/stations"},
		{method: http.MethodGet, target: "/tools/ais/assets/js/display.js?v=1", wantStatus: http.StatusOK, wantType: js, contains: "export function mountDisplay", notContains: "/display/api/"},
		{method: http.MethodGet, target: "/tools/ais/assets/js/runtime.js", wantStatus: http.StatusOK, wantType: js, contains: "export function mount"},
		{method: http.MethodGet, target: "/tools/ais/assets/js/standalone.js", wantStatus: http.StatusOK, wantType: js, contains: `from "./ui.js"`},
		{method: http.MethodHead, target: "/tools/ais/assets/js/htmx.min.js", wantStatus: http.StatusOK, wantType: js},
		{method: http.MethodGet, target: "/tools/ais/assets/leaflet/leaflet-src.esm.js", wantStatus: http.StatusOK, wantType: js, contains: "createMap as map"},
		{method: http.MethodGet, target: "/tools/ais/assets/leaflet/leaflet.css", wantStatus: http.StatusOK, wantType: css, contains: ".leaflet-container"},
		{method: http.MethodHead, target: "/tools/ais/assets/leaflet/images/layers.png", wantStatus: http.StatusOK, wantType: "image/png"},
		{method: http.MethodGet, target: "/tools/ais/assets/leaflet/LICENSE", wantStatus: http.StatusOK, contains: "BSD 2-Clause License"},
		{method: http.MethodPost, target: "/tools/ais/assets/css/ui.css", wantStatus: http.StatusMethodNotAllowed},
		{method: http.MethodGet, target: "/tools/ais/assets/", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, target: "/tools/ais/assets/js/", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, target: "/tools/ais/assets/nope.js", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, target: "/assets/css/ui.css", wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			rec := request(mux, tt.method, tt.target, nil)
			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantType != "" {
				require.Equal(t, tt.wantType, rec.Header().Get("Content-Type"))
			}
			require.Contains(t, rec.Body.String(), tt.contains)
			if tt.notContains != "" {
				require.NotContains(t, rec.Body.String(), tt.notContains)
			}
		})
	}
	// Unmounted paths that escape the asset root are rejected by the handler itself.
	require.Equal(t, http.StatusNotFound, request(u.Assets(), http.MethodGet, "/css/../../ui.go", nil).Code)
}

// TestBundledLeafletIsPinned guards the bundled Leaflet 1.9.4 files against
// accidental edits. The CSS hash equals the official subresource integrity value
// of the release; the module is the unmodified npm dist/leaflet-src.esm.js.
func TestBundledLeafletIsPinned(t *testing.T) {
	u := newUI(t, nestedConfig())
	for path, want := range map[string]string{
		"/leaflet/leaflet.css":        "p4NxAoJBhIIN+hmNHrzRCf9tD/miZyoHS5obTRR9BMY=",
		"/leaflet/leaflet-src.esm.js": "Oe6TRk8R/jhHE35QwNyBifcGxGDjaYnqeHG/fVQPMwY=",
	} {
		rec := request(u.Assets(), http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		sum := sha256.Sum256(rec.Body.Bytes())
		require.Equal(t, want, base64.StdEncoding.EncodeToString(sum[:]), path)
	}
}

// TestComponentStylesAreScoped checks that every rule of the component
// stylesheet, including rules inside at-rule blocks, selects only below a
// component root.
func TestComponentStylesAreScoped(t *testing.T) {
	u := newUI(t, nestedConfig())
	rec := request(u.Assets(), http.MethodGet, "/css/ui.css", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	selectors := cssSelectors(rec.Body.String())
	require.Greater(t, len(selectors), 40)
	scoped := regexp.MustCompile(`^\.ais-(manager|display)(\s|$)`)
	for _, selector := range selectors {
		require.Regexp(t, scoped, selector)
	}
}

// cssSelectors returns the comma-separated selectors of every style rule. It
// skips comments, @import statements, and at-rule preludes, and assumes
// declaration blocks contain no nested rules.
func cssSelectors(css string) []string {
	css = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(css, "")
	var selectors []string
	var prelude strings.Builder
	inDeclarations := false
	for _, r := range css {
		switch {
		case inDeclarations:
			inDeclarations = r != '}'
		case r == ';' || r == '}':
			prelude.Reset()
		case r == '{':
			text := strings.TrimSpace(prelude.String())
			prelude.Reset()
			if strings.HasPrefix(text, "@") {
				continue
			}
			selectors = append(selectors, splitTopLevel(text)...)
			inDeclarations = true
		default:
			prelude.WriteRune(r)
		}
	}
	return selectors
}

// splitTopLevel splits a selector list on commas outside parentheses.
func splitTopLevel(list string) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range list {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(list[start:i]))
				start = i + 1
			}
		}
	}
	return append(parts, strings.TrimSpace(list[start:]))
}

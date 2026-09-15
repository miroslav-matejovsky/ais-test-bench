package ui_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
				`<link rel="stylesheet" href="/tools/ais/assets/css/app.css">`,
				`<script defer src="/tools/ais/assets/js/manager.js"></script>`, `<script defer src="/tools/ais/assets/js/stations.js"></script>`,
				`<script defer src="/tools/ais/assets/js/htmx.min.js"></script>`, `hx-get="/tools/ais/status"`,
				`<div id="ais-manager" class="ais-manager" data-ais-manager data-api-base="/tools/ais/api/">`,
				`href="/tools/ais/api/stations"`, `href="/tools/ais/api/observations"`, `href="/tools/ais/api/messages"`, `href="/tools/ais/api/metadata"`,
				`<input id="vessel-count" name="count" type="number" min="0" step="1" required disabled>`,
				`<input id="speed" name="speed" type="number" min="0" max="100" step="0.01" required disabled>`,
				`id="stations-body"`, `id="station-form"`, `id="station-fields" disabled`, `id="station-conflict" hidden`,
			},
			notContains: []string{`"/api/`, `"/static/`, `"/assets/`, `"/status"`, "display.js"},
		},
		{
			name: "display", handler: display,
			contains: []string{
				"<title>Display", "<h1>Display</h1>",
				`<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css" integrity="sha256-p4NxAoJBhIIN&#43;hmNHrzRCf9tD/miZyoHS5obTRR9BMY=" crossorigin="">`,
				`<script defer src="/tools/ais/assets/js/display.js"></script>`,
				`<div id="ais-display" class="ais-display" data-ais-display data-api-base="/tools/ais/display/api/">`,
				`href="/tools/ais/manager">Open manager</a>`, `id="map"`, `id="station-selection"`, `id="raw-nmea" readonly`,
			},
			notContains: []string{`"/display/api/`, `"/api/`, "htmx", "manager.js"},
		},
		{
			name: "home", handler: u.HomePage(),
			contains: []string{"<title>Home", `<li><a href="/tools/ais/manager">Manager</a>`, `<li><a href="/tools/ais/display">Display</a>`},
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
			for _, shell := range []string{"<!doctype", "<html", "<head", "<body", "<header", "<nav", "<main", "<script", "<link", "<title", "<h1", "hx-get"} {
				require.NotContains(t, strings.ToLower(html), shell)
			}

			for _, id := range []string{"", "1manager", "host manager", `x" onclick="alert(1)`, "café", strings.Repeat("a", 65)} {
				out.Reset()
				require.ErrorContains(t, render(&out, ui.ComponentConfig{ID: id}), "invalid component ID")
				require.Zero(t, out.Len(), "a failed render writes nothing")
			}
		})
	}
}

func TestHostileLinksAreEscaped(t *testing.T) {
	config := nestedConfig()
	config.ManagerURL = `https://example.test/manager?q="><script>alert(1)</script>`
	config.HomeURL = `/tools/ais/?next='onmouseover='alert(1)`
	u := newUI(t, config)
	display, err := u.DisplayPage()
	display = pageHandler(t, display, err)
	var component bytes.Buffer
	require.NoError(t, u.RenderDisplay(&component, ui.ComponentConfig{ID: "display"}))
	for _, html := range []string{request(display, http.MethodGet, "/", nil).Body.String(), component.String()} {
		require.NotContains(t, html, "<script>alert")
		require.NotContains(t, html, `q="><`)
		require.NotContains(t, html, `'onmouseover=`)
	}
	require.Contains(t, component.String(), `href="https://example.test/manager?q=%22%3e%3cscript%3ealert%281%29%3c/script%3e">Open manager</a>`)
}

func TestResources(t *testing.T) {
	u := newUI(t, ui.Config{ManagerAPIBase: "/api/", DisplayAPIBase: "/display/api/", AssetsBase: "/ais-assets/"})
	require.Equal(t, ui.Resources{
		Stylesheets: []ui.Resource{{URL: "/ais-assets/css/app.css"}},
		Scripts:     []ui.Resource{{URL: "/ais-assets/js/manager.js"}, {URL: "/ais-assets/js/stations.js"}},
	}, u.ManagerResources())
	resources := u.DisplayResources()
	require.Equal(t, "https://unpkg.com/leaflet@1.9.4/dist/leaflet.js", resources.Scripts[0].URL)
	require.NotEmpty(t, resources.Scripts[0].Integrity)
	require.Equal(t, ui.Resource{URL: "/ais-assets/js/display.js"}, resources.Scripts[1])
}

func TestAssets(t *testing.T) {
	u := newUI(t, nestedConfig())
	mux := http.NewServeMux()
	mux.Handle("/tools/ais/assets/", http.StripPrefix("/tools/ais/assets", u.Assets()))
	tests := []struct {
		method, target string
		wantStatus     int
		wantType       string
		contains       string
		notContains    string
	}{
		{method: http.MethodGet, target: "/tools/ais/assets/css/app.css", wantStatus: http.StatusOK, wantType: "text/css; charset=utf-8"},
		{method: http.MethodGet, target: "/tools/ais/assets/js/manager.js", wantStatus: http.StatusOK, wantType: "text/javascript; charset=utf-8", contains: "[data-ais-manager]", notContains: `"/api/`},
		{method: http.MethodGet, target: "/tools/ais/assets/js/stations.js", wantStatus: http.StatusOK, wantType: "text/javascript; charset=utf-8", contains: "apiBase", notContains: "/api/stations"},
		{method: http.MethodGet, target: "/tools/ais/assets/js/display.js?v=1", wantStatus: http.StatusOK, wantType: "text/javascript; charset=utf-8", contains: "[data-ais-display]", notContains: "/display/api/"},
		{method: http.MethodHead, target: "/tools/ais/assets/js/htmx.min.js", wantStatus: http.StatusOK, wantType: "text/javascript; charset=utf-8"},
		{method: http.MethodPost, target: "/tools/ais/assets/css/app.css", wantStatus: http.StatusMethodNotAllowed},
		{method: http.MethodGet, target: "/tools/ais/assets/", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, target: "/tools/ais/assets/js/", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, target: "/tools/ais/assets/nope.js", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, target: "/assets/css/app.css", wantStatus: http.StatusNotFound},
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

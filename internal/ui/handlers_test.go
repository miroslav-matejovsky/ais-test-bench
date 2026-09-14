package ui_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/ui"
)

func newPages(t *testing.T, nav ...ui.Link) *ui.Pages {
	t.Helper()
	pages, err := ui.NewPages(slog.New(slog.DiscardHandler), nav)
	require.NoError(t, err)
	return pages
}

func TestPages(t *testing.T) {
	pages := newPages(t, ui.Link{Href: "/manager", Label: "Manager"})

	tests := []struct {
		name        string
		page        func(*ui.Pages, http.ResponseWriter, *http.Request)
		requestType string
		contains    []string
		notContains []string
	}{
		{
			name:     "home links both UIs",
			page:     (*ui.Pages).Home,
			contains: []string{"<title>Home", `href="/manager"`, `href="/display"`},
		},
		{
			name:        "manager page",
			page:        (*ui.Pages).Manager,
			contains:    []string{"<title>Manager", "<h1>Manager</h1>", `hx-get="/status"`, "/static/js/manager.js", `href="/api/messages"`},
			notContains: []string{`href="/display"`},
		},
		{
			name:     "display page",
			page:     (*ui.Pages).Display,
			contains: []string{"<title>Display", "<h1>Display</h1>", `id="map"`, "leaflet@1.9.4", "/static/js/display.js"},
		},
		{
			name:     "status full page",
			page:     (*ui.Pages).Status,
			contains: []string{"<html", "Uptime:"},
		},
		{
			name:        "status history restore gets full page",
			page:        (*ui.Pages).Status,
			requestType: "full",
			contains:    []string{"<html", "Uptime:"},
		},
		{
			name:        "status partial",
			page:        (*ui.Pages).Status,
			requestType: "partial",
			contains:    []string{"Uptime:"},
			notContains: []string{"<html", "<h1>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.requestType != "" {
				req.Header.Set("HX-Request", "true")
				req.Header.Set("HX-Request-Type", tt.requestType)
			}
			rec := httptest.NewRecorder()

			tt.page(pages, rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			body := rec.Body.String()
			for _, s := range tt.contains {
				require.Contains(t, body, s)
			}
			for _, s := range tt.notContains {
				require.NotContains(t, body, s)
			}
		})
	}
}

func TestPagesRenderNavigation(t *testing.T) {
	pages := newPages(t, ui.Link{Href: "/manager", Label: "Manager"}, ui.Link{Href: "/display", Label: "Display"})

	rec := httptest.NewRecorder()
	pages.Display(rec, httptest.NewRequest(http.MethodGet, "/display", nil))

	require.Contains(t, rec.Body.String(), `<a href="/manager">Manager</a>`)
	require.Contains(t, rec.Body.String(), `<a href="/display">Display</a>`)
}

func TestPagesSetVary(t *testing.T) {
	rec := httptest.NewRecorder()
	newPages(t).Status(rec, httptest.NewRequest(http.MethodGet, "/status", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "HX-Request-Type", rec.Header().Get("Vary"))
	require.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
}

func TestStatic(t *testing.T) {
	for path, want := range map[string]int{
		"/static/js/htmx.min.js": http.StatusOK,
		"/static/js/manager.js":  http.StatusOK,
		"/static/nope.js":        http.StatusNotFound,
	} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ui.Static().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, want, rec.Code)
		})
	}
}

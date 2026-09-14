package ui_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulation"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/ui"
)

func TestHandler(t *testing.T) {
	simulator, err := simulation.New(time.Now(), 1)
	require.NoError(t, err)
	handler, err := ui.NewHandler(slog.New(slog.DiscardHandler), simulator)
	require.NoError(t, err)

	tests := []struct {
		name        string
		path        string
		requestType string
		wantStatus  int
		contains    []string
		notContains []string
	}{
		{
			name:       "home links both UIs",
			path:       "/",
			wantStatus: http.StatusOK,
			contains:   []string{"<title>Home", `href="/manager"`, `href="/display"`},
		},
		{
			name:       "manager page",
			path:       "/manager",
			wantStatus: http.StatusOK,
			contains:   []string{"<title>Manager", "<h1>Manager</h1>", `hx-get="/status"`},
		},
		{
			name:       "display page",
			path:       "/display",
			wantStatus: http.StatusOK,
			contains:   []string{"<title>Display", "<h1>Display</h1>", `id="map"`, "leaflet@1.9.4", "/static/js/display.js"},
		},
		{
			name:       "status full page",
			path:       "/status",
			wantStatus: http.StatusOK,
			contains:   []string{"<html", "Uptime:"},
		},
		{
			name:        "status history restore gets full page",
			path:        "/status",
			requestType: "full",
			wantStatus:  http.StatusOK,
			contains:    []string{"<html", "Uptime:"},
		},
		{
			name:        "status partial",
			path:        "/status",
			requestType: "partial",
			wantStatus:  http.StatusOK,
			contains:    []string{"Uptime:"},
			notContains: []string{"<html", "<h1>"},
		},
		{
			name:       "htmx served",
			path:       "/static/js/htmx.min.js",
			wantStatus: http.StatusOK,
			contains:   []string{"htmx"},
		},
		{
			name:       "unknown path",
			path:       "/nope",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.requestType != "" {
				req.Header.Set("HX-Request", "true")
				req.Header.Set("HX-Request-Type", tt.requestType)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
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

func TestHandlerSetsVaryOnPages(t *testing.T) {
	simulator, err := simulation.New(time.Now(), 1)
	require.NoError(t, err)
	handler, err := ui.NewHandler(slog.New(slog.DiscardHandler), simulator)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/status", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "HX-Request-Type", rec.Header().Get("Vary"))
	require.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
}

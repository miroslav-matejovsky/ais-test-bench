package display_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/display"
)

func TestStandaloneRoutes(t *testing.T) {
	u := newUpstream(t)
	client, err := display.NewClient(u.server.URL + "/api/")
	require.NoError(t, err)
	handler, err := display.NewStandaloneHandler(display.StandaloneConfig{
		Client: client, Logger: slog.New(slog.DiscardHandler), ManagerURL: u.server.URL + "/manager",
	})
	require.NoError(t, err)

	tests := []struct {
		method       string
		path         string
		wantStatus   int
		wantLocation string
		contains     []string
		notContains  []string
	}{
		{method: http.MethodGet, path: "/display", wantStatus: http.StatusOK, contains: []string{"<h1>Display</h1>", `<a href="/display">Display</a>`, `data-api-base="/display/api/"`, `<script type="module" src="/assets/js/standalone.js">`, `href="` + u.server.URL + `/manager">Open manager</a>`}, notContains: []string{`href="/manager"`, "/static/"}},
		{method: http.MethodHead, path: "/display", wantStatus: http.StatusOK},
		{method: http.MethodPost, path: "/display", wantStatus: http.StatusMethodNotAllowed},
		{method: http.MethodGet, path: "/display/api/observations?stations=all", wantStatus: http.StatusOK, contains: []string{`"simulationId":"run-1"`, `"targets":[{`}},
		{method: http.MethodGet, path: "/display/api/stations/s1/receptions?simulationId=run-1", wantStatus: http.StatusOK, contains: []string{`"stationId":"s1"`}},
		{method: http.MethodGet, path: "/assets/js/display.js", wantStatus: http.StatusOK, contains: []string{"observations?stations="}, notContains: []string{"/display/api/"}},
		{method: http.MethodGet, path: "/assets/js/", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, path: "/", wantStatus: http.StatusFound, wantLocation: "/display"},
		{method: http.MethodGet, path: "/?stations=s1", wantStatus: http.StatusFound, wantLocation: "/display?stations=s1"},
		{method: http.MethodPost, path: "/display/api/observations", wantStatus: http.StatusMethodNotAllowed},
		{method: http.MethodGet, path: "/display/api/vessels", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, path: "/manager", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, path: "/api/vessels", wantStatus: http.StatusNotFound},
		{method: http.MethodPut, path: "/api/vessels", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, path: "/api/metadata", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, path: "/static/js/display.js", wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			require.Equal(t, tt.wantStatus, rec.Code)
			require.Equal(t, tt.wantLocation, rec.Header().Get("Location"))
			for _, s := range tt.contains {
				require.Contains(t, rec.Body.String(), s)
			}
			for _, s := range tt.notContains {
				require.NotContains(t, rec.Body.String(), s)
			}
		})
	}
	require.Equal(t, []string{"/api/observations?stations=all", "/api/stations/s1/receptions?simulationId=run-1"}, u.requested(), "only the display API reads the simulator")

	_, err = display.NewStandaloneHandler(display.StandaloneConfig{Client: client, ManagerURL: "javascript:alert(1)"})
	require.ErrorContains(t, err, "ManagerURL")
	_, err = display.NewStandaloneHandler(display.StandaloneConfig{})
	require.ErrorContains(t, err, "client is required")
}

func TestRunServesPageWithoutSimulator(t *testing.T) {
	stopped := httptest.NewServer(http.NotFoundHandler())
	stopped.Close()
	client, err := display.NewClient(stopped.URL + "/api/")
	require.NoError(t, err)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- display.Run(ctx, ln, display.StandaloneConfig{Client: client, Logger: slog.New(slog.DiscardHandler)})
	}()

	// The listener is already bound, so requests queue until serving starts.
	base := "http://" + ln.Addr().String()
	status, body := request(t, base+"/")
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "<h1>Display</h1>")
	status, body = request(t, base+"/display/api/observations")
	require.Equal(t, http.StatusServiceUnavailable, status)
	require.Contains(t, body, "simulator unavailable")

	cancel()
	require.NoError(t, <-done)
}

func request(t *testing.T, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode, string(body)
}

package main

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

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantAddr    string
		wantManager string
		wantBase    string
		wantErr     bool
	}{
		{name: "defaults", args: nil, wantAddr: "localhost:8081", wantManager: "http://localhost:8000/manager"},
		{name: "explicit", args: []string{"-addr", "127.0.0.1:9001", "-simulator-url", "http://127.0.0.1:9000", "-base-path", "/map"}, wantAddr: "127.0.0.1:9001", wantManager: "http://127.0.0.1:9000/manager", wantBase: "/map"},
		{name: "https with root path", args: []string{"-simulator-url", "https://simulator.example:8443/"}, wantAddr: "localhost:8081", wantManager: "https://simulator.example:8443/manager"},
		{name: "prefixed simulator", args: []string{"-simulator-url", "http://localhost:8000/tools/ais/"}, wantAddr: "localhost:8081", wantManager: "http://localhost:8000/tools/ais/manager"},
		{name: "missing host", args: []string{"-addr", ":8081"}, wantErr: true},
		{name: "invalid port", args: []string{"-addr", "localhost:0"}, wantErr: true},
		{name: "base path with trailing slash", args: []string{"-base-path", "/map/"}, wantErr: true},
		{name: "simulator URL without scheme", args: []string{"-simulator-url", "localhost:8000"}, wantErr: true},
		{name: "unsupported scheme", args: []string{"-simulator-url", "ws://localhost:8000"}, wantErr: true},
		{name: "malformed simulator URL", args: []string{"-simulator-url", "http://local host"}, wantErr: true},
		{name: "simulator URL dot segment", args: []string{"-simulator-url", "http://localhost:8000/a/.."}, wantErr: true},
		{name: "simulator URL query", args: []string{"-simulator-url", "http://localhost:8000?x=1"}, wantErr: true},
		{name: "simulator URL user info", args: []string{"-simulator-url", "http://me@localhost:8000"}, wantErr: true},
		{name: "unknown flag", args: []string{"-port", "8081"}, wantErr: true},
		{name: "positional argument", args: []string{"localhost:8081"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseArgs(tt.args, io.Discard)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantAddr, cfg.addr)
			require.NotNil(t, cfg.client)
			require.Equal(t, tt.wantManager, cfg.managerURL)
			require.Equal(t, tt.wantBase, cfg.basePath)
		})
	}
}

func TestServeRunsDisplayUntilCancelled(t *testing.T) {
	stopped := httptest.NewServer(http.NotFoundHandler())
	stopped.Close()
	client, err := display.NewClient(stopped.URL + "/api/")
	require.NoError(t, err)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	cfg := config{client: client, managerURL: stopped.URL + "/manager", basePath: "/map"}
	go func() { done <- serve(ctx, ln, cfg, slog.New(slog.DiscardHandler)) }()

	// The listener is already bound, so requests queue until serving starts.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String()+"/map/display", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), `href="`+stopped.URL+`/manager">Open manager</a>`)

	cancel()
	require.NoError(t, <-done, "an unreachable simulator does not fail the display")
}

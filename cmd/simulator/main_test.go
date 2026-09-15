package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    config
		wantErr bool
	}{
		{name: "default", args: nil, want: config{addr: "localhost:8000"}},
		{name: "explicit", args: []string{"-addr", "localhost:9000", "-base-path", "/sim"}, want: config{addr: "localhost:9000", basePath: "/sim"}},
		{name: "equals form", args: []string{"-addr=127.0.0.1:1"}, want: config{addr: "127.0.0.1:1"}},
		{name: "missing host", args: []string{"-addr", ":8000"}, wantErr: true},
		{name: "empty address", args: []string{"-addr", ""}, wantErr: true},
		{name: "invalid port", args: []string{"-addr", "localhost:65536"}, wantErr: true},
		{name: "base path with trailing slash", args: []string{"-base-path", "/sim/"}, wantErr: true},
		{name: "base path dot segment", args: []string{"-base-path", "/sim/.."}, wantErr: true},
		{name: "unknown flag", args: []string{"-port", "8000"}, wantErr: true},
		{name: "positional argument", args: []string{"localhost:8000"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args, io.Discard)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestServeRunsSimulatorUntilCancelled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln, config{basePath: "/sim"}, slog.New(slog.DiscardHandler)) }()

	// The listener is already bound, so requests queue until serving starts.
	base := "http://" + ln.Addr().String() + "/sim"
	require.Contains(t, get(t, base+"/manager"), "<h1>Manager</h1>")
	require.Contains(t, get(t, base+"/api/stations"), `"stations":[{`, "the demonstration stations are configured")

	cancel()
	require.NoError(t, <-done)
}

func get(t *testing.T, url string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	return string(body)
}

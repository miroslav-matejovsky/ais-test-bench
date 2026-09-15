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
		{name: "explicit", args: []string{"-addr", "localhost:9000", "-base-path", "/tools/ais"}, want: config{addr: "localhost:9000", basePath: "/tools/ais"}},
		{name: "equals form", args: []string{"-addr=127.0.0.1:1"}, want: config{addr: "127.0.0.1:1"}},
		{name: "missing host", args: []string{"-addr", ":8000"}, wantErr: true},
		{name: "invalid port", args: []string{"-addr", "localhost:0"}, wantErr: true},
		{name: "base path with trailing slash", args: []string{"-base-path", "/tools/"}, wantErr: true},
		{name: "relative base path", args: []string{"-base-path", "tools"}, wantErr: true},
		{name: "unknown flag", args: []string{"-simulator-url", "http://localhost:8000"}, wantErr: true},
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

func TestRunRejectsInvalidArgumentsBeforeListening(t *testing.T) {
	err := run(t.Context(), []string{"-base-path", "/"}, io.Discard, slog.New(slog.DiscardHandler))
	require.ErrorContains(t, err, "-base-path")
}

func TestServeRunsBenchUntilCancelled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln, config{basePath: "/tools/ais"}, slog.New(slog.DiscardHandler)) }()

	// The listener is already bound, so requests queue until serving starts.
	base := "http://" + ln.Addr().String() + "/tools/ais"
	for path, want := range map[string]string{
		"/manager":                  "<h1>Manager</h1>",
		"/display":                  "<h1>Display</h1>",
		"/display/api/observations": `"stations":[{`,
	} {
		require.Contains(t, get(t, base+path), want, path)
	}

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

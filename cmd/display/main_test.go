package main

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantAddr   string
		wantOrigin string
		wantErr    bool
	}{
		{name: "defaults", args: nil, wantAddr: "localhost:8081", wantOrigin: "http://localhost:8000"},
		{name: "explicit", args: []string{"-addr", "127.0.0.1:9001", "-simulator-url", "http://127.0.0.1:9000"}, wantAddr: "127.0.0.1:9001", wantOrigin: "http://127.0.0.1:9000"},
		{name: "https with root path", args: []string{"-simulator-url", "https://simulator.example:8443/"}, wantAddr: "localhost:8081", wantOrigin: "https://simulator.example:8443"},
		{name: "missing host", args: []string{"-addr", ":8081"}, wantErr: true},
		{name: "invalid port", args: []string{"-addr", "localhost:0"}, wantErr: true},
		{name: "simulator URL without scheme", args: []string{"-simulator-url", "localhost:8000"}, wantErr: true},
		{name: "unsupported scheme", args: []string{"-simulator-url", "ws://localhost:8000"}, wantErr: true},
		{name: "malformed simulator URL", args: []string{"-simulator-url", "http://local host"}, wantErr: true},
		{name: "simulator URL path", args: []string{"-simulator-url", "http://localhost:8000/api"}, wantErr: true},
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
			require.Equal(t, tt.wantOrigin, cfg.client.Origin())
		})
	}
}

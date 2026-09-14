package main

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAddr(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{name: "default", args: nil, want: defaultAddr},
		{name: "explicit", args: []string{"-addr", "localhost:9000"}, want: "localhost:9000"},
		{name: "equals form", args: []string{"-addr=127.0.0.1:1"}, want: "127.0.0.1:1"},
		{name: "ipv6 loopback", args: []string{"-addr", "[::1]:65535"}, want: "[::1]:65535"},
		{name: "missing host", args: []string{"-addr", ":8080"}, wantErr: true},
		{name: "missing port", args: []string{"-addr", "localhost"}, wantErr: true},
		{name: "empty port", args: []string{"-addr", "localhost:"}, wantErr: true},
		{name: "port zero", args: []string{"-addr", "localhost:0"}, wantErr: true},
		{name: "port too large", args: []string{"-addr", "localhost:65536"}, wantErr: true},
		{name: "port not a number", args: []string{"-addr", "localhost:http"}, wantErr: true},
		{name: "unknown flag", args: []string{"-port", "8080"}, wantErr: true},
		{name: "positional argument", args: []string{"localhost:8080"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAddr(tt.args, io.Discard)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

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
		{name: "default", args: nil, want: "localhost:8000"},
		{name: "explicit", args: []string{"-addr", "localhost:9000"}, want: "localhost:9000"},
		{name: "equals form", args: []string{"-addr=127.0.0.1:1"}, want: "127.0.0.1:1"},
		{name: "missing host", args: []string{"-addr", ":8000"}, wantErr: true},
		{name: "empty address", args: []string{"-addr", ""}, wantErr: true},
		{name: "invalid port", args: []string{"-addr", "localhost:65536"}, wantErr: true},
		{name: "unknown flag", args: []string{"-port", "8000"}, wantErr: true},
		{name: "positional argument", args: []string{"localhost:8000"}, wantErr: true},
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

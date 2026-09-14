package main

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePort(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    int
		wantErr bool
	}{
		{name: "default", args: nil, want: defaultPort},
		{name: "explicit", args: []string{"-port", "9000"}, want: 9000},
		{name: "equals form", args: []string{"-port=1"}, want: 1},
		{name: "max", args: []string{"-port", "65535"}, want: 65535},
		{name: "zero", args: []string{"-port", "0"}, wantErr: true},
		{name: "too large", args: []string{"-port", "65536"}, wantErr: true},
		{name: "not a number", args: []string{"-port", "abc"}, wantErr: true},
		{name: "unknown flag", args: []string{"-config", "x.yaml"}, wantErr: true},
		{name: "positional argument", args: []string{"9000"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePort(tt.args, io.Discard)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

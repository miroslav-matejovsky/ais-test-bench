package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/cli"
)

func TestValidateListenAddr(t *testing.T) {
	tests := []struct {
		addr    string
		wantErr bool
	}{
		{addr: "localhost:9000"},
		{addr: "127.0.0.1:1"},
		{addr: "[::1]:65535"},
		{addr: "", wantErr: true},
		{addr: ":8000", wantErr: true},
		{addr: "localhost", wantErr: true},
		{addr: "localhost:", wantErr: true},
		{addr: "localhost:0", wantErr: true},
		{addr: "localhost:65536", wantErr: true},
		{addr: "localhost:http", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			err := cli.ValidateListenAddr(tt.addr)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

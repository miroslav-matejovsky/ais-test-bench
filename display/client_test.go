package display_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/display"
)

func TestNewClientValidatesSimulatorURL(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{url: "http://localhost:8000"},
		{url: "http://localhost:8000/"},
		{url: "https://simulator.example"},
		{url: "http://127.0.0.1:1"},
		{url: "http://[::1]:65535"},
		{url: "", wantErr: true},
		{url: "localhost:8000", wantErr: true},
		{url: "ftp://localhost:8000", wantErr: true},
		{url: "http://", wantErr: true},
		{url: "http://:8000", wantErr: true},
		{url: "http://localhost:", wantErr: true},
		{url: "http://localhost:0", wantErr: true},
		{url: "http://localhost:65536", wantErr: true},
		{url: "http://user:secret@localhost:8000", wantErr: true},
		{url: "http://localhost:8000/api", wantErr: true},
		{url: "http://localhost:8000?x=1", wantErr: true},
		{url: "http://localhost:8000/?", wantErr: true},
		{url: "http://localhost:8000#top", wantErr: true},
		{url: "http://local host:8000", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			client, err := display.NewClient(tt.url)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, client)
		})
	}
}

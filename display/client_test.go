package display_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/display"
)

func TestNewClientValidatesSimulatorAPIBase(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{url: "http://localhost:8000/api/"},
		{url: "http://localhost:8000/"},
		{url: "https://simulator.example/tools/ais/api/"},
		{url: "HTTP://127.0.0.1:1/api/"},
		{url: "http://[::1]:65535/api/"},
		{url: "", wantErr: true},
		{url: "localhost:8000/api/", wantErr: true},
		{url: "ftp://localhost:8000/api/", wantErr: true},
		{url: "http:///api/", wantErr: true},
		{url: "http://:8000/api/", wantErr: true},
		{url: "http://localhost:/api/", wantErr: true},
		{url: "http://localhost:0/api/", wantErr: true},
		{url: "http://localhost:65536/api/", wantErr: true},
		{url: "http://user:secret@localhost:8000/api/", wantErr: true},
		{url: "http://localhost:8000", wantErr: true},
		{url: "http://localhost:8000/api", wantErr: true},
		{url: "http://localhost:8000//api/", wantErr: true},
		{url: "http://localhost:8000/a/../api/", wantErr: true},
		{url: "http://localhost:8000/a%2Fb/", wantErr: true},
		{url: "http://localhost:8000/api/?x=1", wantErr: true},
		{url: "http://localhost:8000/api/?", wantErr: true},
		{url: "http://localhost:8000/api/#top", wantErr: true},
		{url: "http://local host:8000/api/", wantErr: true},
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

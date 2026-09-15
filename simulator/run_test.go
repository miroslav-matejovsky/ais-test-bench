package simulator

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/simulation"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

func get(t *testing.T, url string) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	return body
}

func TestServeManagerAndAPIBelowBasePath(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ln, StandaloneConfig{Simulation: DemoConfig(), Logger: slog.New(slog.DiscardHandler), BasePath: "/tools/ais"})
	}()

	// The listener is already bound, so requests queue until Serve accepts them.
	base := "http://" + ln.Addr().String() + "/tools/ais"
	manager := string(get(t, base+"/manager"))
	require.Contains(t, manager, "<h1>Manager</h1>")
	require.Contains(t, manager, `data-api-base="/tools/ais/api/"`)
	var metadata simulatorapi.Metadata
	require.NoError(t, json.Unmarshal(get(t, base+"/api/metadata"), &metadata))
	var fleet simulatorapi.Fleet
	require.NoError(t, json.Unmarshal(get(t, base+"/api/vessels"), &fleet))
	require.NotEmpty(t, metadata.SimulationID)
	require.Equal(t, metadata.SimulationID, fleet.SimulationID)
	require.Len(t, fleet.Vessels, 1)

	cancel()
	require.NoError(t, <-done)
}

// failingListener fails every Accept and reports Close on closed.
type failingListener struct {
	closed chan struct{}
}

func (l failingListener) Accept() (net.Conn, error) { return nil, errors.New("accept failed") }
func (l failingListener) Close() error              { close(l.closed); return nil }
func (l failingListener) Addr() net.Addr            { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

func TestServeClosesListenerOnFailure(t *testing.T) {
	discard := slog.New(slog.DiscardHandler)
	tests := []struct {
		name    string
		config  StandaloneConfig
		wantErr string
		wantIs  error
	}{
		{name: "invalid engine config", config: StandaloneConfig{Logger: discard}, wantIs: simulation.ErrInvalid},
		{name: "invalid base path", config: StandaloneConfig{Simulation: runtimeConfig().Simulation, Logger: discard, BasePath: "/tools/"}, wantErr: "base path"},
		{name: "serving", config: StandaloneConfig{Simulation: runtimeConfig().Simulation, Logger: discard}, wantErr: "accept failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ln := failingListener{closed: make(chan struct{})}

			// Serve returns only after the pacing loop has exited.
			err := Serve(t.Context(), ln, tt.config)

			if tt.wantIs != nil {
				require.ErrorIs(t, err, tt.wantIs)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
			<-ln.closed
		})
	}
}

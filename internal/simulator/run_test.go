package simulator_test

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

	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulator"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
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

func TestRunServesManagerAndAPI(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- simulator.Run(ctx, slog.New(slog.DiscardHandler), ln)
	}()

	// The listener is already bound, so requests queue until Serve accepts them.
	base := "http://" + ln.Addr().String()
	require.Contains(t, string(get(t, base+"/manager")), "<h1>Manager</h1>")
	var metadata simulatorapi.Metadata
	require.NoError(t, json.Unmarshal(get(t, base+"/api/metadata"), &metadata))
	var fleet simulatorapi.Fleet
	require.NoError(t, json.Unmarshal(get(t, base+"/api/vessels"), &fleet))
	require.NotEmpty(t, metadata.SimulationID)
	require.Equal(t, metadata.SimulationID, fleet.SimulationID)
	require.Len(t, fleet.Vessels, simulation.InitialVesselCount)

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

func TestServeFailureClosesListenerAndStopsEngine(t *testing.T) {
	_, driver, handler := newHandler(t)
	ln := failingListener{closed: make(chan struct{})}

	// Serve returns only after the tick loop has exited.
	err := simulator.Serve(t.Context(), slog.New(slog.DiscardHandler), ln, driver, handler)

	require.ErrorContains(t, err, "accept failed")
	select {
	case <-ln.closed:
	default:
		t.Fatal("listener was not closed")
	}
}

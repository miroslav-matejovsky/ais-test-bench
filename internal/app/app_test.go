package app_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/app"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/display"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
)

func do(t *testing.T, method, url, body string) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	return data
}

func getJSON[T any](t *testing.T, url string) T {
	t.Helper()
	var value T
	require.NoError(t, json.Unmarshal(do(t, http.MethodGet, url, ""), &value))
	return value
}

func TestRunServesCombinedComponents(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx, slog.New(slog.DiscardHandler), ln)
	}()

	// The listener is already bound, so requests queue until serving starts.
	base := "http://" + ln.Addr().String()
	home := string(do(t, http.MethodGet, base+"/", ""))
	require.Contains(t, home, `href="/manager"`)
	require.Contains(t, home, `href="/display"`)
	manager := string(do(t, http.MethodGet, base+"/manager", ""))
	require.Contains(t, manager, "<h1>Manager</h1>")
	require.Contains(t, manager, `<a href="/display">Display</a>`)
	require.Contains(t, string(do(t, http.MethodGet, base+"/display", "")), "<h1>Display</h1>")

	metadata := getJSON[simulatorapi.Metadata](t, base+"/api/metadata")
	fleet := getJSON[simulatorapi.Fleet](t, base+"/api/vessels")
	projection := getJSON[display.Fleet](t, base+"/display/api/vessels")
	require.Equal(t, metadata.SimulationID, fleet.SimulationID)
	require.Equal(t, metadata.SimulationID, projection.SimulationID, "public API and display client read one engine")
	require.Len(t, projection.Vessels, 1)
	require.Equal(t, fleet.Vessels[0].MMSI, projection.Vessels[0].MMSI)
	require.NotNil(t, projection.Vessels[0].Latitude)

	do(t, http.MethodPut, base+"/api/vessels", `{"count":3}`)
	require.Len(t, getJSON[display.Fleet](t, base+"/display/api/vessels").Vessels, 3)
	do(t, http.MethodPut, base+"/api/vessels", `{"count":0}`)
	require.Empty(t, getJSON[display.Fleet](t, base+"/display/api/vessels").Vessels)

	// One engine emits each report once: sequences are consecutive.
	history := getJSON[simulatorapi.History](t, base+"/api/messages")
	for i, message := range history.Messages {
		require.Equal(t, uint64(i+1), message.Sequence)
	}

	cancel()
	require.NoError(t, <-done)
}

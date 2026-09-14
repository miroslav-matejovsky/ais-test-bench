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

	"github.com/miroslav-matejovsky/ais-testbench/internal/app"
	"github.com/miroslav-matejovsky/ais-testbench/internal/display"
	"github.com/miroslav-matejovsky/ais-testbench/internal/simulatorapi"
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
	upstream := getJSON[simulatorapi.Observations](t, base+"/api/observations")
	projection := getJSON[display.Observations](t, base+"/display/api/observations?stations=all")
	require.Equal(t, metadata.SimulationID, upstream.SimulationID)
	require.Equal(t, metadata.SimulationID, projection.SimulationID, "public API and display client read one engine")
	require.Len(t, projection.Stations, len(upstream.Stations))
	require.NotEmpty(t, projection.Stations, "demonstration stations are configured")
	for _, target := range projection.Targets {
		require.Equal(t, target.MMSI, target.Report.MMSI)
		require.NotEmpty(t, target.Report.Sentence)
	}

	first := projection.Stations[0]
	selected := getJSON[display.Observations](t, base+"/display/api/observations?stations="+first.ID)
	require.Equal(t, []string{first.ID}, selected.Selection)
	page := getJSON[display.ReceptionPage](t, base+"/display/api/stations/"+first.ID+"/receptions?simulationId="+metadata.SimulationID)
	require.Equal(t, first.ID, page.StationID)
	require.True(t, page.Tail)

	do(t, http.MethodPut, base+"/api/vessels", `{"count":3}`)
	do(t, http.MethodPut, base+"/api/vessels", `{"count":0}`)
	require.Equal(t, metadata.SimulationID, getJSON[display.Observations](t, base+"/display/api/observations").SimulationID)

	// One engine emits each report once: sequences are consecutive.
	history := getJSON[simulatorapi.History](t, base+"/api/messages")
	for i, message := range history.Messages {
		require.Equal(t, uint64(i+1), message.Sequence)
	}

	cancel()
	require.NoError(t, <-done)
}

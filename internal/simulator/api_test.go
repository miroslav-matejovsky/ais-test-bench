package simulator_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	nmea "github.com/adrianmo/go-nmea"
	"github.com/stretchr/testify/require"

	simdriver "github.com/miroslav-matejovsky/ais-test-bench/internal/simulation"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulator"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

// newHandler returns the engine, its driver, and the standalone handler.
func newHandler(t *testing.T) (*simulation.Simulator, *simdriver.Driver, http.Handler) {
	t.Helper()
	sim, err := simulation.New("run-1", time.Now(), 1)
	require.NoError(t, err)
	driver := simdriver.NewDriver(sim)
	handler, err := simulator.NewHandler(slog.New(slog.DiscardHandler), driver)
	require.NoError(t, err)
	return sim, driver, handler
}

func serve(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	var value T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &value))
	return value
}

func TestFleetCarriesExactNMEA(t *testing.T) {
	sim, _, handler := newHandler(t)

	rec := serve(handler, http.MethodGet, "/api/vessels", "")

	require.Contains(t, rec.Body.String(), `\r\n"`)
	fleet := decode[simulatorapi.Fleet](t, rec)
	require.Equal(t, "run-1", fleet.SimulationID)
	require.Equal(t, simulation.MessageLimit, fleet.MessageLimit)
	require.Equal(t, 1, fleet.MessageCount)
	require.Len(t, fleet.Vessels, 1)
	vessel := fleet.Vessels[0]
	require.Equal(t, "cargo", vessel.TypeID)
	require.Equal(t, uint64(1), vessel.Report.Sequence)
	require.Equal(t, sim.History().Messages[0].Sentence, vessel.Report.Sentence)
	require.True(t, strings.HasSuffix(vessel.Report.Sentence, "\r\n"))
	_, err := nmea.Parse(vessel.Report.Sentence)
	require.NoError(t, err)
}

func TestCountChangesFleetAndHistory(t *testing.T) {
	sim, _, handler := newHandler(t)

	grown := decode[simulatorapi.Fleet](t, serve(handler, http.MethodPut, "/api/vessels", `{"count":3}`))
	require.Len(t, grown.Vessels, 3)
	require.NoError(t, sim.Advance(time.Now().Add(time.Hour)))

	history := decode[simulatorapi.History](t, serve(handler, http.MethodGet, "/api/messages", ""))
	require.Equal(t, "run-1", history.SimulationID)
	require.Len(t, history.Messages, 6)
	require.NotNil(t, history.OldestSequence)
	require.NotNil(t, history.LatestSequence)
	require.Equal(t, uint64(1), *history.OldestSequence)
	require.Equal(t, uint64(6), *history.LatestSequence)

	emptied := serve(handler, http.MethodPut, "/api/vessels", `{"count":0}`)
	require.Contains(t, emptied.Body.String(), `"vessels":[]`)
	require.Empty(t, decode[simulatorapi.Fleet](t, emptied).Vessels)
	require.Len(t, decode[simulatorapi.History](t, serve(handler, http.MethodGet, "/api/messages", "")).Messages, 6)
}

func TestMetadataAPI(t *testing.T) {
	sim, _, handler := newHandler(t)

	rec := serve(handler, http.MethodGet, "/api/metadata", "")

	decode[simulatorapi.Metadata](t, rec)
	startedAt, err := json.Marshal(sim.Metadata().StartedAt)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"simulationId": "run-1",
		"startedAt": `+string(startedAt)+`,
		"vesselTypes": [{"id": "cargo", "name": "Cargo vessel"}],
		"supportedMessageTypes": [1],
		"settings": {
			"initialVesselCount": 1, "maxVessels": 100, "tickIntervalMs": 1000, "messageIntervalMs": 1000, "messageHistoryLimit": 1000,
			"speedKnots": {"min": 6, "max": 15.9},
			"spawnBounds": {"south": 52, "north": 52.04, "west": 3.94, "east": 4}
		}
	}`, rec.Body.String())
}

func TestCountRejectsInvalidRequests(t *testing.T) {
	sim, _, handler := newHandler(t)
	before := sim.Fleet()

	for _, body := range []string{`{}`, `null`, `{"count":null}`, `{"count":-1}`, `{"count":101}`, `{"count":1.5}`, `{"count":"2"}`, `{"count":2,"extra":true}`, `{"count":2} {}`, `{`, strings.Repeat(" ", 1025) + `{"count":2}`} {
		t.Run(body[:min(len(body), 40)], func(t *testing.T) {
			rec := serve(handler, http.MethodPut, "/api/vessels", body)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, before, sim.Fleet())
		})
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/vessels", strings.NewReader(`{"count":2}`)))
	require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	require.Equal(t, before, sim.Fleet())
}

func TestStandaloneRoutes(t *testing.T) {
	_, _, handler := newHandler(t)

	tests := []struct {
		method      string
		path        string
		wantStatus  int
		contains    []string
		notContains []string
	}{
		{method: http.MethodGet, path: "/manager", wantStatus: http.StatusOK, contains: []string{"<h1>Manager</h1>", `<a href="/manager">Manager</a>`}, notContains: []string{`href="/display"`}},
		{method: http.MethodGet, path: "/status", wantStatus: http.StatusOK, contains: []string{"Uptime:"}},
		{method: http.MethodGet, path: "/static/js/manager.js", wantStatus: http.StatusOK},
		{method: http.MethodGet, path: "/display", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, path: "/display/api/vessels", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, path: "/api/unknown", wantStatus: http.StatusNotFound},
		{method: http.MethodPost, path: "/api/vessels", wantStatus: http.StatusMethodNotAllowed},
		{method: http.MethodDelete, path: "/api/messages", wantStatus: http.StatusMethodNotAllowed},
		{method: http.MethodPut, path: "/api/metadata", wantStatus: http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := serve(handler, tt.method, tt.path, "")
			require.Equal(t, tt.wantStatus, rec.Code)
			for _, s := range tt.contains {
				require.Contains(t, rec.Body.String(), s)
			}
			for _, s := range tt.notContains {
				require.NotContains(t, rec.Body.String(), s)
			}
		})
	}

	rec := serve(handler, http.MethodGet, "/", "")
	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/manager", rec.Header().Get("Location"))
}

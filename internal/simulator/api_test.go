package simulator_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	nmea "github.com/adrianmo/go-nmea"
	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/ais"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simdriver"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulator"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

// start is a custom past virtual start, half a second before a new year.
var start = time.Date(1999, 12, 31, 23, 59, 59, 500000000, time.UTC)

// testClock is a manual real clock. Its heartbeat never fires, so virtual time
// advances only when a command settles elapsed time.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *testClock) NewTicker(time.Duration) (<-chan time.Time, func()) {
	return nil, func() {}
}

// fixture is one engine served by the standalone handler through a driver on a
// manual clock.
type fixture struct {
	sim     *simulation.Simulator
	clock   *testClock
	driver  *simdriver.Driver
	handler http.Handler
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	sim, err := simulation.New(simulation.Config{
		ID: "run-1", StartTime: start, Seed: 1, InitialVesselCount: 1, Speed: 1,
		Transmitter: simulation.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1},
	})
	require.NoError(t, err)
	clock := &testClock{now: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)}
	driver := simdriver.NewDriver(sim, clock)
	handler, err := simulator.NewHandler(slog.New(slog.DiscardHandler), driver)
	require.NoError(t, err)
	return fixture{sim: sim, clock: clock, driver: driver, handler: handler}
}

// state returns every observable engine read.
func (f fixture) state() [3]any {
	return [3]any{f.sim.Fleet(), f.sim.History(), f.sim.Metadata()}
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
	f := newFixture(t)

	rec := serve(f.handler, http.MethodGet, "/api/vessels", "")

	require.Contains(t, rec.Body.String(), `\r\n"`)
	fleet := decode[simulatorapi.Fleet](t, rec)
	require.Equal(t, "run-1", fleet.SimulationID)
	require.Equal(t, simulation.MessageLimit, fleet.MessageLimit)
	require.Equal(t, 1, fleet.MessageCount)
	require.Len(t, fleet.Vessels, 1)
	vessel := fleet.Vessels[0]
	require.Equal(t, "cargo", vessel.TypeID)
	require.Equal(t, uint64(1), vessel.Report.Sequence)
	require.Equal(t, f.sim.History().Messages[0].Sentence, vessel.Report.Sentence)
	require.True(t, strings.HasSuffix(vessel.Report.Sentence, "\r\n"))
	_, err := nmea.Parse(vessel.Report.Sentence)
	require.NoError(t, err)
}

func TestCountChangesFleetAndHistory(t *testing.T) {
	f := newFixture(t)

	grown := decode[simulatorapi.Fleet](t, serve(f.handler, http.MethodPut, "/api/vessels", `{"count":3}`))
	require.Len(t, grown.Vessels, 3)

	// Reads never settle elapsed time; the next command does.
	f.clock.Add(time.Second)
	require.Len(t, decode[simulatorapi.History](t, serve(f.handler, http.MethodGet, "/api/messages", "")).Messages, 3)
	decode[simulatorapi.Fleet](t, serve(f.handler, http.MethodPut, "/api/vessels", `{"count":3}`))

	history := decode[simulatorapi.History](t, serve(f.handler, http.MethodGet, "/api/messages", ""))
	require.Equal(t, "run-1", history.SimulationID)
	require.Len(t, history.Messages, 6)
	require.NotNil(t, history.OldestSequence)
	require.NotNil(t, history.LatestSequence)
	require.Equal(t, uint64(1), *history.OldestSequence)
	require.Equal(t, uint64(6), *history.LatestSequence)

	emptied := serve(f.handler, http.MethodPut, "/api/vessels", `{"count":0}`)
	require.Contains(t, emptied.Body.String(), `"vessels":[]`)
	require.Empty(t, decode[simulatorapi.Fleet](t, emptied).Vessels)
	require.Len(t, decode[simulatorapi.History](t, serve(f.handler, http.MethodGet, "/api/messages", "")).Messages, 6)
}

func TestMetadataAPI(t *testing.T) {
	f := newFixture(t)

	rec := serve(f.handler, http.MethodGet, "/api/metadata", "")

	decode[simulatorapi.Metadata](t, rec)
	require.JSONEq(t, `{
		"simulationId": "run-1",
		"startedAt": "1999-12-31T23:59:59.5Z",
		"time": {"now": "1999-12-31T23:59:59.5Z", "elapsedMs": 0, "speed": 1, "paused": false},
		"vesselTypes": [{"id": "cargo", "name": "Cargo vessel"}],
		"supportedMessageTypes": [1],
		"settings": {
			"maxStations": 16,
			"transmitter": {"powerWatts":12.5,"heightMeters":10,"gainDbi":2,"feederLossDb":1},
			"reception": {"siteLossDb":15,"pathExponent":3.5,"effectiveEarthRadiusFactor":1.3333333333333333,
				"channelAFrequencyMhz":161.975,"channelBFrequencyMhz":162.025,"horizonTaperStart":0.8,
				"zeroProbabilityMarginDb":-12,"referenceProbability":0.8,"fullProbabilityMarginDb":6,"coverageThresholds":[0.9,0.5]},
			"observation": {"receptionHistoryLimit":1000,"targetLimit":1000,"recentReceptionLimit":50,
				"freshAgeMs":10000,"staleAgeMs":60000,"expiryAgeMs":600000,"rateWindowMs":60000},
			"initialVesselCount": 1, "maxVessels": 100, "tickIntervalMs": 1000, "messageIntervalMs": 1000,
			"pacingIntervalMs": 100, "messageHistoryLimit": 1000,
			"speedKnots": {"min": 6, "max": 15.9},
			"speed": {"min": 0.01, "max": 100, "step": 0.01},
			"spawnBounds": {"south": 52, "north": 52.04, "west": 3.94, "east": 4}
		}
	}`, rec.Body.String())
}

func TestTimeAPI(t *testing.T) {
	f := newFixture(t)

	// Speed changes settle elapsed time at the previous speed.
	f.clock.Add(1500 * time.Millisecond)
	metadata := decode[simulatorapi.Metadata](t, serve(f.handler, http.MethodPut, "/api/time", `{"speed":2}`))
	require.Equal(t, simulatorapi.TimeState{Now: start.Add(1500 * time.Millisecond), ElapsedMs: 1500, Speed: 2}, metadata.Time)

	// Count creation uses the current virtual time of the custom date, and the
	// AIS second agrees with the report timestamp after the rate change.
	f.clock.Add(time.Second)
	fleet := decode[simulatorapi.Fleet](t, serve(f.handler, http.MethodPut, "/api/vessels", `{"count":2}`))
	created := fleet.Vessels[1].Report
	require.Equal(t, time.Date(2000, 1, 1, 0, 0, 3, 0, time.UTC), created.Timestamp)
	position, err := ais.DecodePosition(created.Sentence)
	require.NoError(t, err)
	require.Equal(t, created.Timestamp.Second(), *position.Second)

	paused := serve(f.handler, http.MethodPut, "/api/time", `{"speed":0}`)
	require.Contains(t, paused.Body.String(), `"speed":0,"paused":true`)
	require.True(t, decode[simulatorapi.Metadata](t, paused).Time.Paused)
	f.clock.Add(time.Hour)
	metadata = decode[simulatorapi.Metadata](t, serve(f.handler, http.MethodPut, "/api/time", `{"speed":0.25}`))
	require.Equal(t, simulatorapi.TimeState{Now: start.Add(3500 * time.Millisecond), ElapsedMs: 3500, Speed: 0.25}, metadata.Time)
}

func TestTimeRejectsInvalidRequests(t *testing.T) {
	f := newFixture(t)
	f.clock.Add(5 * time.Second)
	before := f.state()

	for _, body := range []string{
		`{}`, `null`, `{"speed":null}`, `{"speed":"2"}`, `{"speed":true}`, `{"speed":-1}`, `{"speed":100.01}`,
		`{"speed":0.015}`, `{"speed":1e-9}`, `{"speed":2,"extra":true}`, `{"speed":2} {}`, `{"speed":2}x`, `{`,
		strings.Repeat(" ", 1025) + `{"speed":2}`,
	} {
		t.Run(body[:min(len(body), 40)], func(t *testing.T) {
			rec := serve(f.handler, http.MethodPut, "/api/time", body)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, before, f.state(), "a rejected request neither settles time nor changes state")
		})
	}

	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/time", strings.NewReader(`{"speed":2}`)))
	require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	require.Equal(t, before, f.state())
}

func TestCountRejectsInvalidRequests(t *testing.T) {
	f := newFixture(t)
	f.clock.Add(5 * time.Second)
	before := f.state()

	for _, body := range []string{`{}`, `null`, `{"count":null}`, `{"count":-1}`, `{"count":101}`, `{"count":1.5}`, `{"count":"2"}`, `{"count":2,"extra":true}`, `{"count":2} {}`, `{`, strings.Repeat(" ", 1025) + `{"count":2}`} {
		t.Run(body[:min(len(body), 40)], func(t *testing.T) {
			rec := serve(f.handler, http.MethodPut, "/api/vessels", body)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, before, f.state())
		})
	}

	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/vessels", strings.NewReader(`{"count":2}`)))
	require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	require.Equal(t, before, f.state())
}

func TestCommandFailureReturnsServerError(t *testing.T) {
	f := newFixture(t)
	f.clock.Add(simdriver.MaxCatchUp + time.Second)
	before := f.state()

	for _, tt := range []struct{ path, body string }{
		{path: "/api/vessels", body: `{"count":2}`},
		{path: "/api/time", body: `{"speed":2}`},
	} {
		rec := serve(f.handler, http.MethodPut, tt.path, tt.body)
		require.Equal(t, http.StatusInternalServerError, rec.Code, tt.path)
		require.Equal(t, before, f.state())
	}
}

func TestStandaloneRoutes(t *testing.T) {
	f := newFixture(t)

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
		{method: http.MethodGet, path: "/display/api/observations", wantStatus: http.StatusNotFound},
		{method: http.MethodGet, path: "/api/unknown", wantStatus: http.StatusNotFound},
		{method: http.MethodPost, path: "/api/vessels", wantStatus: http.StatusMethodNotAllowed},
		{method: http.MethodDelete, path: "/api/messages", wantStatus: http.StatusMethodNotAllowed},
		{method: http.MethodPut, path: "/api/metadata", wantStatus: http.StatusMethodNotAllowed},
		{method: http.MethodGet, path: "/api/time", wantStatus: http.StatusMethodNotAllowed},
		{method: http.MethodPost, path: "/api/time", wantStatus: http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := serve(f.handler, tt.method, tt.path, "")
			require.Equal(t, tt.wantStatus, rec.Code)
			for _, s := range tt.contains {
				require.Contains(t, rec.Body.String(), s)
			}
			for _, s := range tt.notContains {
				require.NotContains(t, rec.Body.String(), s)
			}
		})
	}

	rec := serve(f.handler, http.MethodGet, "/", "")
	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/manager", rec.Header().Get("Location"))
}

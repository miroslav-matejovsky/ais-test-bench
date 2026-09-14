package display_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/ais"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/display"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
)

var reportTime = time.Date(2026, 9, 14, 12, 0, 5, 0, time.UTC)

// noPositionSentence is a type 1 report for MMSI 200000002 with unavailable
// longitude and latitude, 8.0 knots, course 90.0, heading 90, and second 5.
const noPositionSentence = "!AIVDM,1,1,,A,12vg20PP1@<tSF0l4Q@3Q2l:0000,0*65\r\n"

func sentence(t *testing.T, mmsi uint32, latitude, longitude float64) string {
	t.Helper()
	s, err := ais.EncodePosition(ais.Position{MMSI: mmsi, Latitude: latitude, Longitude: longitude, Speed: 10.5, Course: 45, Heading: 45, UpdatedAt: reportTime})
	require.NoError(t, err)
	return s
}

// fixtureVessel carries identity and NMEA only; there are no decoded values.
func fixtureVessel(t *testing.T, mmsi uint32, latitude, longitude float64) simulatorapi.Vessel {
	return simulatorapi.Vessel{MMSI: mmsi, Name: "Vessel 1", TypeID: "cargo", Report: simulatorapi.Report{
		Sequence: 1, Timestamp: reportTime, Sentence: sentence(t, mmsi, latitude, longitude),
	}}
}

// upstream is a fixture simulator API server.
type upstream struct {
	server *httptest.Server

	mu       sync.Mutex
	fleet    simulatorapi.Fleet
	metadata simulatorapi.Metadata
	override http.HandlerFunc // Serves every request when set.
	paths    []string
}

func newUpstream(t *testing.T) *upstream {
	u := &upstream{
		fleet: simulatorapi.Fleet{
			SimulationID: "run-1", UpdatedAt: reportTime, MessageCount: 1, MessageLimit: 1000,
			Vessels: []simulatorapi.Vessel{fixtureVessel(t, 200000001, 52.01, 3.95)},
		},
		metadata: simulatorapi.Metadata{
			SimulationID: "run-1", StartedAt: reportTime,
			VesselTypes:           []simulatorapi.VesselType{{ID: "cargo", Name: "Cargo vessel"}},
			SupportedMessageTypes: []int{1},
			Settings: simulatorapi.Settings{
				InitialVesselCount: 1, MaxVessels: 100, TickIntervalMs: 1000, MessageIntervalMs: 1000, MessageHistoryLimit: 1000,
				SpeedKnots:  simulatorapi.SpeedRange{Min: 6, Max: 15.9},
				SpawnBounds: simulatorapi.SpawnBounds{South: 52, North: 52.04, West: 3.94, East: 4},
			},
		},
	}
	u.server = httptest.NewServer(http.HandlerFunc(u.serve))
	t.Cleanup(u.server.Close)
	return u
}

func (u *upstream) serve(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	u.paths = append(u.paths, r.URL.Path)
	fleet, metadata, override := u.fleet, u.metadata, u.override
	u.mu.Unlock()
	if override != nil {
		override(w, r)
		return
	}
	var value any
	switch r.URL.Path {
	case "/api/vessels":
		value = fleet
	case "/api/metadata":
		value = metadata
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (u *upstream) set(change func(*upstream)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	change(u)
}

func (u *upstream) requestedPaths() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return slices.Sorted(slices.Values(u.paths))
}

func newAPI(t *testing.T, u *upstream) http.Handler {
	t.Helper()
	client, err := display.NewClient(u.server.URL)
	require.NoError(t, err)
	t.Cleanup(client.CloseIdleConnections)
	return display.NewAPI(slog.New(slog.DiscardHandler), client)
}

func fetch(handler http.Handler) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/display/api/vessels", nil))
	return rec
}

func decodeFleet(t *testing.T, rec *httptest.ResponseRecorder) display.Fleet {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	var fleet display.Fleet
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &fleet))
	return fleet
}

func TestVesselsProjectsNMEA(t *testing.T) {
	u := newUpstream(t)

	fleet := decodeFleet(t, fetch(newAPI(t, u)))

	require.Equal(t, "run-1", fleet.SimulationID)
	require.Equal(t, reportTime, fleet.UpdatedAt)
	require.Equal(t, simulatorapi.SpawnBounds{South: 52, North: 52.04, West: 3.94, East: 4}, fleet.SpawnBounds)
	require.Len(t, fleet.Vessels, 1)
	vessel := fleet.Vessels[0]
	require.Equal(t, uint32(200000001), vessel.MMSI)
	require.Equal(t, "Vessel 1", vessel.Name)
	require.Equal(t, "cargo", vessel.TypeID)
	require.Equal(t, "Cargo vessel", vessel.TypeName)
	require.InDelta(t, 52.01, *vessel.Latitude, 1.0/600000)
	require.InDelta(t, 3.95, *vessel.Longitude, 1.0/600000)
	require.InDelta(t, 10.5, *vessel.Speed, 1e-9)
	require.InDelta(t, 45.0, *vessel.Course, 1e-9)
	require.Equal(t, 45, *vessel.Heading)
	require.Equal(t, reportTime, vessel.UpdatedAt)
	require.Equal(t, []string{"/api/metadata", "/api/vessels"}, u.requestedPaths())
}

func TestVesselsFollowNMEAAndRuns(t *testing.T) {
	u := newUpstream(t)
	handler := newAPI(t, u)
	decodeFleet(t, fetch(handler))

	u.set(func(u *upstream) { u.fleet.Vessels[0].Report.Sentence = sentence(t, 200000001, -33.5, -70.25) })
	moved := decodeFleet(t, fetch(handler)).Vessels[0]
	require.InDelta(t, -33.5, *moved.Latitude, 1.0/600000)
	require.InDelta(t, -70.25, *moved.Longitude, 1.0/600000)

	u.set(func(u *upstream) {
		u.fleet.SimulationID, u.metadata.SimulationID = "run-2", "run-2"
		u.fleet.Vessels = []simulatorapi.Vessel{fixtureVessel(t, 200000001, 52.02, 3.96)}
		u.fleet.Vessels[0].Name = "Restarted"
	})
	restarted := decodeFleet(t, fetch(handler))
	require.Equal(t, "run-2", restarted.SimulationID)
	require.Equal(t, "Restarted", restarted.Vessels[0].Name)
}

func TestVesselsWithoutPositionAreNull(t *testing.T) {
	u := newUpstream(t)
	u.set(func(u *upstream) {
		u.fleet.Vessels = append(u.fleet.Vessels, simulatorapi.Vessel{MMSI: 200000002, Name: "Vessel 2", TypeID: "tanker", Report: simulatorapi.Report{
			Sequence: 2, Timestamp: reportTime, Sentence: noPositionSentence,
		}})
	})

	rec := fetch(newAPI(t, u))

	fleet := decodeFleet(t, rec)
	require.Len(t, fleet.Vessels, 2)
	vessel := fleet.Vessels[1]
	require.Nil(t, vessel.Latitude)
	require.Nil(t, vessel.Longitude)
	require.InDelta(t, 8.0, *vessel.Speed, 1e-9)
	require.Equal(t, "tanker", vessel.TypeName, "unknown type IDs are shown as their name")
	require.Contains(t, rec.Body.String(), `"latitude":null,"longitude":null`)
}

func TestVesselsEmptyFleet(t *testing.T) {
	u := newUpstream(t)
	u.set(func(u *upstream) { u.fleet.Vessels = []simulatorapi.Vessel{} })

	rec := fetch(newAPI(t, u))

	require.Empty(t, decodeFleet(t, rec).Vessels)
	require.Contains(t, rec.Body.String(), `"vessels":[]`)
	require.Equal(t, []string{"/api/metadata", "/api/vessels"}, u.requestedPaths())
}

func respond(status int, contentType, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func TestVesselsUpstreamFailures(t *testing.T) {
	tests := []struct {
		name       string
		change     func(*upstream)
		wantStatus int
	}{
		{"unreachable", func(u *upstream) { u.server.Close() }, http.StatusServiceUnavailable},
		{"restart between reads", func(u *upstream) { u.metadata.SimulationID = "run-2" }, http.StatusServiceUnavailable},
		{"error status", func(u *upstream) { u.override = respond(http.StatusInternalServerError, "text/plain", "boom") }, http.StatusBadGateway},
		{"wrong content type", func(u *upstream) { u.override = respond(http.StatusOK, "text/plain", "{}") }, http.StatusBadGateway},
		{"invalid JSON", func(u *upstream) { u.override = respond(http.StatusOK, "application/json", "{") }, http.StatusBadGateway},
		{"data after object", func(u *upstream) { u.override = respond(http.StatusOK, "application/json", "{} {}") }, http.StatusBadGateway},
		{"oversized body", func(u *upstream) {
			u.override = respond(http.StatusOK, "application/json", `{"name":"`+strings.Repeat("x", 2<<20)+`"}`)
		}, http.StatusBadGateway},
		{"missing simulation id", func(u *upstream) { u.fleet.SimulationID = "" }, http.StatusBadGateway},
		{"missing vessels", func(u *upstream) { u.fleet.Vessels = nil }, http.StatusBadGateway},
		{"no vessel types", func(u *upstream) { u.metadata.VesselTypes = nil }, http.StatusBadGateway},
		{"invalid spawn bounds", func(u *upstream) { u.metadata.Settings.SpawnBounds.North = 51 }, http.StatusBadGateway},
		{"duplicate MMSI", func(u *upstream) { u.fleet.Vessels = append(u.fleet.Vessels, u.fleet.Vessels[0]) }, http.StatusBadGateway},
		{"missing type id", func(u *upstream) { u.fleet.Vessels[0].TypeID = "" }, http.StatusBadGateway},
		{"missing report time", func(u *upstream) { u.fleet.Vessels[0].Report.Timestamp = time.Time{} }, http.StatusBadGateway},
		{"corrupt NMEA", func(u *upstream) {
			u.fleet.Vessels[0].Report.Sentence = strings.Replace(u.fleet.Vessels[0].Report.Sentence, ",A,", ",B,", 1)
		}, http.StatusBadGateway},
		{"payload MMSI mismatch", func(u *upstream) { u.fleet.Vessels[0].MMSI = 200000009 }, http.StatusBadGateway},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := newUpstream(t)
			handler := newAPI(t, u)
			u.set(tt.change)

			rec := fetch(handler)

			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			require.NotContains(t, rec.Body.String(), `"vessels"`)
		})
	}
}

func TestVesselsConcurrentReaders(t *testing.T) {
	u := newUpstream(t)
	handler := newAPI(t, u)
	want := fetch(handler).Body.String()

	bodies := make(chan string, 8)
	var wg sync.WaitGroup
	for range cap(bodies) {
		wg.Go(func() { bodies <- fetch(handler).Body.String() })
	}
	wg.Wait()
	close(bodies)

	for body := range bodies {
		require.Equal(t, want, body)
	}
}

func TestVesselsCancellationStopsUpstreamReads(t *testing.T) {
	u := newUpstream(t)
	handler := newAPI(t, u)
	started, stopped := make(chan string, 2), make(chan string, 2)
	u.set(func(u *upstream) {
		u.override = func(_ http.ResponseWriter, r *http.Request) {
			started <- r.URL.Path
			<-r.Context().Done()
			stopped <- r.URL.Path
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/display/api/vessels", nil))
		done <- rec
	}()

	<-started
	<-started
	cancel()

	require.ElementsMatch(t, []string{"/api/metadata", "/api/vessels"}, []string{<-stopped, <-stopped})
	require.Equal(t, http.StatusServiceUnavailable, (<-done).Code)
}

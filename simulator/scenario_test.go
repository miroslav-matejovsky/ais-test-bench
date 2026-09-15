package simulator

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/display"
	"github.com/miroslav-matejovsky/ais-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/ais-testbench/simulation"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

// fixedClock is a real-time clock that never moves; tests step the engine.
type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }
func (fixedClock) NewTicker(time.Duration) (<-chan time.Time, func()) {
	return make(chan time.Time), func() {}
}

// Station IDs of scenarioStations, in definition order.
const (
	coast   = "station-1" // Sensitive high site: receives every report.
	harbour = "station-2" // Channel B disabled, lower gain: receives channel A only.
	deaf    = "station-3" // Degraded receiver at the coast site: margin far below zero.
	far     = "station-4" // 1,300 km away: outside the radio horizon.
)

func scenarioStations() []simulation.StationDefinition {
	channel := simulation.ReceiverChannel{Enabled: true, SensitivityDBm: -125}
	site := func(name string, latitude, longitude float64) simulation.StationDefinition {
		return simulation.StationDefinition{
			Name: name, Latitude: latitude, Longitude: longitude, Enabled: true,
			AntennaHeightMeters: 40, ReceiveGainDBi: 10, FeederLossDB: 0, ChannelA: channel, ChannelB: channel,
		}
	}
	h := site("Harbour", 52.00, 4.00)
	h.AntennaHeightMeters, h.ReceiveGainDBi, h.FeederLossDB = 25, 0, 2
	h.ChannelA.SensitivityDBm, h.ChannelB.SensitivityDBm = -110, -110
	h.ChannelB.Enabled = false
	d := site("Deaf", 52.02, 3.97)
	d.ReceiveGainDBi, d.FeederLossDB = -10, 30
	for _, c := range []*simulation.ReceiverChannel{&d.ChannelA, &d.ChannelB} {
		c.SensitivityDBm, c.NoisePenaltyDB = -80, 40
	}
	return []simulation.StationDefinition{site("Coast", 52.02, 3.97), h, d, site("Far", 40, 4)}
}

// scenario wires one engine to the simulator API over HTTP and a display API
// reading it through the same client both process arrangements use.
type scenario struct {
	engine   *simulation.Simulator
	upstream *httptest.Server
	display  http.Handler

	mu    sync.Mutex
	paths []string // Simulator paths the display read.
}

func newScenario(t *testing.T) *scenario {
	t.Helper()
	engine, err := simulation.New(simulation.Config{
		ID: "scenario", StartTime: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), Seed: 3, InitialVesselCount: 3, Speed: 1,
		Transmitter: simulation.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1},
		Stations:    scenarioStations(),
	})
	require.NoError(t, err)
	logger := slog.New(slog.DiscardHandler)
	api := http.StripPrefix("/api", newAPI(logger, &Simulator{driver: simdriver.NewDriver(engine, fixedClock{})}))
	s := &scenario{engine: engine}
	s.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Scenario-Truth") == "" {
			s.mu.Lock()
			s.paths = append(s.paths, r.URL.Path)
			s.mu.Unlock()
		}
		api.ServeHTTP(w, r)
	}))
	t.Cleanup(s.upstream.Close)
	client, err := display.NewClient(s.upstream.URL + "/api/")
	require.NoError(t, err)
	t.Cleanup(client.CloseIdleConnections)
	handler, err := display.NewHandler(display.Config{Client: client, Logger: logger})
	require.NoError(t, err)
	s.display = http.StripPrefix("/display/api", handler)
	return s
}

func (s *scenario) advance(t *testing.T, d time.Duration) {
	t.Helper()
	_, err := s.engine.Advance(t.Context(), d)
	require.NoError(t, err)
}

func (s *scenario) setEnabled(t *testing.T, id string, enabled bool) {
	t.Helper()
	set := s.engine.Stations()
	i := slices.IndexFunc(set.Stations, func(st simulation.Station) bool { return st.ID == id })
	definition := set.Stations[i].Definition
	definition.Enabled = enabled
	_, err := s.engine.UpdateStation(set.Revision, id, definition)
	require.NoError(t, err)
}

// get requests the display API and returns its status and body.
func (s *scenario) get(t *testing.T, target string) (int, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.display.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
	return rec.Code, rec.Body.Bytes()
}

func (s *scenario) observations(t *testing.T, stations string) display.Observations {
	t.Helper()
	status, body := s.get(t, "/display/api/observations?stations="+stations)
	require.Equal(t, http.StatusOK, status, string(body))
	var o display.Observations
	require.NoError(t, json.Unmarshal(body, &o))
	return o
}

// truth reads the simulator's generated fleet over HTTP, outside the display.
func (s *scenario) truth(t *testing.T) simulatorapi.Fleet {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.upstream.URL+"/api/vessels", nil)
	require.NoError(t, err)
	req.Header.Set("X-Scenario-Truth", "1")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	var fleet simulatorapi.Fleet
	require.NoError(t, json.Unmarshal(body, &fleet))
	return fleet
}

func station(o display.Observations, id string) display.Station {
	return o.Stations[slices.IndexFunc(o.Stations, func(s display.Station) bool { return s.ID == id })]
}

func provenance(target display.Target, id string) *display.TargetStation {
	i := slices.IndexFunc(target.Stations, func(s display.TargetStation) bool { return s.StationID == id })
	if i < 0 {
		return nil
	}
	return &target.Stations[i]
}

// TestStationDifferencesReachDisplay drives one seeded engine with explicit
// stations and checks, through the simulator and display HTTP APIs, what each
// station received. Every link here has decode probability 1 or 0, so no
// assertion depends on a random draw.
func TestStationDifferencesReachDisplay(t *testing.T) {
	s := newScenario(t)
	s.advance(t, 4*time.Second) // Creation reports plus four ticks: 15 transmissions.
	fleet := s.truth(t)
	require.Len(t, fleet.Vessels, 3)
	latest := make(map[uint32]simulatorapi.Report)
	for _, v := range fleet.Vessels {
		latest[v.MMSI] = v.Report
	}

	all := s.observations(t, "all")
	require.Len(t, all.Targets, 3, "one aggregate target per MMSI despite overlapping receivers")
	for _, target := range all.Targets {
		require.Equal(t, latest[target.MMSI].Sentence, target.Report.Sentence)
		require.True(t, provenance(target, coast).Chosen)
		harbourSeen := provenance(target, harbour)
		require.NotNil(t, harbourSeen, "harbour receives channel A reports of every vessel")
		require.Equal(t, target.Report.Channel == "A", harbourSeen.Chosen)
	}

	t.Run("different capabilities", func(t *testing.T) {
		power := map[uint64]map[string]float64{}
		for _, r := range all.RecentReceptions {
			if power[r.TransmissionSequence] == nil {
				power[r.TransmissionSequence] = map[string]float64{}
			}
			power[r.TransmissionSequence][r.StationID] = r.Signal.EstimatedPowerDBm
		}
		shared := 0
		for _, byStation := range power {
			if h, ok := byStation[harbour]; ok {
				require.NotEqual(t, byStation[coast], h, "one report, different estimated power per station")
				shared++
			}
		}
		require.Positive(t, shared)
	})

	t.Run("channel B failure", func(t *testing.T) {
		h := station(all, harbour).Counters
		require.Zero(t, h.ChannelB.Received)
		require.Equal(t, h.ChannelB.Opportunities, h.ChannelDisabled)
		require.Equal(t, h.ChannelA.Opportunities, h.ChannelA.Received)
		c := station(all, coast).Counters
		require.Equal(t, uint64(15), c.Received)
		require.Positive(t, c.ChannelB.Received)
		for _, r := range s.observations(t, harbour).RecentReceptions {
			require.Equal(t, "A", r.Channel)
		}
	})

	t.Run("outside all coverage", func(t *testing.T) {
		require.Empty(t, s.observations(t, deaf+","+far).Targets, "the truth fleet has vessels no selected station received")
		require.Equal(t, uint64(15), station(all, far).Counters.OutsideHorizon)
		require.Equal(t, uint64(15), station(all, deaf).Counters.Opportunities)
		require.Zero(t, station(all, deaf).Counters.Received)
	})

	t.Run("history", func(t *testing.T) {
		status, body := s.get(t, "/display/api/stations/"+coast+"/receptions?simulationId=scenario&limit=200")
		require.Equal(t, http.StatusOK, status, string(body))
		var page display.ReceptionPage
		require.NoError(t, json.Unmarshal(body, &page))
		require.Len(t, page.Receptions, 15)
	})

	// Lost update and receiver diversity: coast misses two ticks, harbour does not.
	before := s.observations(t, coast).Targets
	s.setEnabled(t, coast, false)
	s.advance(t, 2*time.Second)
	fleet = s.truth(t)

	t.Run("lost update keeps last received position", func(t *testing.T) {
		after := s.observations(t, coast).Targets
		require.Len(t, after, len(before))
		for i, target := range after {
			require.Equal(t, before[i].Report.Sentence, target.Report.Sentence)
			require.Equal(t, before[i].Report.ReceivedAt, target.Report.ReceivedAt)
			require.Equal(t, before[i].AgeMs+2000, target.AgeMs, "age follows virtual time")
			require.NotEqual(t, fleet.Vessels[i].Report.Sentence, target.Report.Sentence, "the vessel moved")
		}
	})

	t.Run("receiver diversity", func(t *testing.T) {
		for i, target := range s.observations(t, "all").Targets {
			require.Equal(t, harbour, target.Report.StationID, "a later channel A report reached harbour")
			old := provenance(target, coast)
			require.False(t, old.Chosen)
			require.Equal(t, before[i].Report.ReceivedAt, old.ReceivedAt, "coast keeps its accurate old receive time")
			require.False(t, old.StationEnabled)
		}
	})

	t.Run("re-enable receives only later reports", func(t *testing.T) {
		s.setEnabled(t, coast, true)
		s.advance(t, time.Second)
		fleet := s.truth(t)
		o := s.observations(t, coast)
		for i, target := range o.Targets {
			require.Equal(t, fleet.Vessels[i].Report.Sentence, target.Report.Sentence)
		}
		require.Equal(t, uint64(6), station(o, coast).Counters.StationDisabled)
		require.Equal(t, uint64(18), station(o, coast).Counters.Received)
	})

	t.Run("deleted station", func(t *testing.T) {
		_, err := s.engine.RemoveStation(s.engine.Stations().Revision, harbour)
		require.NoError(t, err)
		status, body := s.get(t, "/display/api/observations?stations="+harbour)
		require.Equal(t, http.StatusNotFound, status, string(body))
		status, body = s.get(t, "/display/api/stations/"+harbour+"/receptions?simulationId=scenario")
		require.Equal(t, http.StatusNotFound, status, string(body))
		for _, target := range s.observations(t, "all").Targets {
			require.Nil(t, provenance(target, harbour))
		}
	})

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, path := range s.paths {
		require.True(t, path == "/api/observations" || strings.HasSuffix(path, "/receptions"), "display read %s", path)
	}
}

// BenchmarkDisplayObservations measures one display request at maximum
// occupancy through both HTTP hops: engine snapshot, simulator JSON encoding,
// display bounded read, validation, NMEA decoding, and display JSON encoding.
func BenchmarkDisplayObservations(b *testing.B) {
	definitions := make([]simulation.StationDefinition, 0, simulation.MaxStations)
	for range simulation.MaxStations {
		definitions = append(definitions, scenarioStations()[0])
	}
	engine, err := simulation.New(simulation.Config{
		ID: "benchmark", StartTime: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), Seed: 3, InitialVesselCount: 0, Speed: 1,
		Transmitter: simulation.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1},
		Stations:    definitions,
	})
	require.NoError(b, err)
	for range simulation.TargetLimit / simulation.MaxVessels {
		_, err = engine.SetCount(simulation.MaxVessels)
		require.NoError(b, err)
		_, err = engine.SetCount(0)
		require.NoError(b, err)
	}
	logger := slog.New(slog.DiscardHandler)
	upstream := httptest.NewServer(newAPI(logger, &Simulator{driver: simdriver.NewDriver(engine, fixedClock{})}))
	b.Cleanup(upstream.Close)
	client, err := display.NewClient(upstream.URL + "/")
	require.NoError(b, err)
	api, err := display.NewHandler(display.Config{Client: client, Logger: logger})
	require.NoError(b, err)
	handler := http.StripPrefix("/display/api", api)

	var size int
	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/display/api/observations?stations=all", nil))
		if rec.Code != http.StatusOK {
			b.Fatal(rec.Body.String())
		}
		size = rec.Body.Len()
	}
	b.ReportMetric(float64(size), "display-bytes")
}

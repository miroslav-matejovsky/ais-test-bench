package display_test

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/ais"
	"github.com/miroslav-matejovsky/ais-testbench/internal/display"
	"github.com/miroslav-matejovsky/ais-testbench/internal/simulatorapi"
)

var (
	start = time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	now   = start.Add(20 * time.Second)
)

// noPositionSentence is a type 1 report on channel A for MMSI 200000002 with
// unavailable longitude and latitude, 8.0 knots, course 90.0, heading 90, and
// UTC second 5.
const noPositionSentence = "!AIVDM,1,1,,A,12vg20PP1@<tSF0l4Q@3Q2l:0000,0*65\r\n"

func sentence(t *testing.T, mmsi uint32, latitude float64, channel ais.Channel, at time.Time) string {
	t.Helper()
	s, err := ais.EncodePosition(ais.Position{MMSI: mmsi, Latitude: latitude, Longitude: 3.95, Speed: 10.5, Course: 45, Heading: 45, UpdatedAt: at}, channel)
	require.NoError(t, err)
	return s
}

func metadata() simulatorapi.Metadata {
	return simulatorapi.Metadata{
		SimulationID: "run-1", StartedAt: start,
		Time:                  simulatorapi.TimeState{Now: now, ElapsedMs: 20000, Speed: 1},
		VesselTypes:           []simulatorapi.VesselType{{ID: "cargo", Name: "Cargo vessel"}},
		SupportedMessageTypes: []int{1},
		Settings: simulatorapi.Settings{
			MaxStations: 16,
			Transmitter: simulatorapi.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1},
			Reception: simulatorapi.ReceptionModel{
				SiteLossDB: 10, PathExponent: 2.6, EffectiveEarthRadiusFactor: 4.0 / 3,
				ChannelAFrequencyMHz: 161.975, ChannelBFrequencyMHz: 162.025, HorizonTaperStart: 0.8,
				ZeroProbabilityMarginDB: -6, ReferenceProbability: 0.5, FullProbabilityMarginDB: 6, CoverageThresholds: []float64{0.9, 0.5},
			},
			Observation: simulatorapi.ObservationSettings{
				ReceptionHistoryLimit: 1000, TargetLimit: 1000, RecentReceptionLimit: 50,
				FreshAgeMs: 10000, StaleAgeMs: 60000, ExpiryAgeMs: 600000, RateWindowMs: 60000,
			},
			InitialVesselCount: 1, MaxVessels: 100, TickIntervalMs: 1000, MessageIntervalMs: 1000, PacingIntervalMs: 100, MessageHistoryLimit: 1000,
			SpeedKnots:  simulatorapi.SpeedRange{Min: 6, Max: 15.9},
			Speed:       simulatorapi.SpeedLimits{Min: 0.01, Max: 100, Step: 0.01},
			SpawnBounds: simulatorapi.SpawnBounds{South: 52, North: 52.04, West: 3.94, East: 4},
		},
	}
}

func fixtureStation(id string, c simulatorapi.ReceptionCounters) simulatorapi.StationObservation {
	channel := simulatorapi.ReceiverChannel{Enabled: true, SensitivityDBm: -110}
	coverage := make([]simulatorapi.Coverage, 0, 4)
	for _, name := range []string{"A", "B"} {
		for _, threshold := range []float64{0.9, 0.5} {
			coverage = append(coverage, simulatorapi.Coverage{
				Channel: name, Threshold: threshold, MinRadiusMeters: 1000, MaxRadiusMeters: 20000,
				Geometry: simulatorapi.Geometry{Type: "MultiPolygon", Coordinates: [][][][2]float64{{{{3.9, 51.9}, {4.1, 51.9}, {4.1, 52.1}, {3.9, 52.1}, {3.9, 51.9}}}}},
			})
		}
	}
	var ratio *float64
	if c.Opportunities > 0 {
		ratio = new(float64(c.Received) / float64(c.Opportunities))
	}
	return simulatorapi.StationObservation{
		Station: simulatorapi.Station{
			ID: id, ConfigRevision: 1, RFRevision: 1, CreatedAt: start, RFUpdatedAt: start, Coverage: coverage,
			Definition: simulatorapi.StationDefinition{
				Name: "Station " + id, Latitude: 52, Longitude: 4, Enabled: true,
				AntennaHeightMeters: 30, ReceiveGainDBi: 3, FeederLossDB: 2, ChannelA: channel, ChannelB: channel,
				ShadowSectors: []simulatorapi.ShadowSector{},
			},
		},
		Counters: c, ReceiveRatio: ratio,
		Recent: simulatorapi.RecentCounters{
			DurationMs: 20000, Opportunities: c.Opportunities, Received: c.Received, ReceiveRatio: ratio,
			OpportunityRate: new(float64(c.Opportunities) / 20), ReceptionRate: new(float64(c.Received) / 20),
		},
	}
}

func fixtureReception(station simulatorapi.StationObservation, sequence, transmission uint64, mmsi uint32, channel string, at time.Time, sentence string) simulatorapi.Reception {
	d := station.Definition
	return simulatorapi.Reception{
		StationID: station.ID, Sequence: sequence, TransmissionSequence: transmission, MMSI: mmsi, Channel: channel,
		Timestamp: at, Sentence: sentence, VesselName: "Vessel " + strconv.Itoa(int(mmsi%10)), VesselTypeID: "cargo",
		ConfigRevision: 1, RFRevision: 1,
		Receiver: simulatorapi.ReceiverSnapshot{
			Name: d.Name, Latitude: d.Latitude, Longitude: d.Longitude, AntennaHeightMeters: d.AntennaHeightMeters,
			ReceiveGainDBi: d.ReceiveGainDBi, FeederLossDB: d.FeederLossDB, Channel: d.ChannelA,
		},
		Link: simulatorapi.ReceptionLink{
			DistanceMeters: 5000, BearingDegrees: new(90.0), HorizonMeters: 40000,
			ReceivedPowerDBm: -80, EffectiveSensitivityDBm: -110, MarginDB: 30, Probability: 1,
		},
	}
}

// scenario is a consistent simulator state from which the fixture derives
// snapshots and history pages. Three transmissions are evaluated at stations
// s1 and s2:
//
//	tx1 +5s  MMSI 200000002 channel A, no position: s2 receives (s2/1)
//	tx2 +10s MMSI 200000001 channel B, 52.00N:      s1 (s1/1) and s2 (s2/2) receive
//	tx3 +18s MMSI 200000001 channel A, 52.01N:      s1 receives (s1/2), s2 misses
type scenario struct {
	stations      []simulatorapi.StationObservation
	receptions    []simulatorapi.Reception // In transmission order.
	transmissions uint64
	// retainedFrom is the oldest retained history sequence per station; 1 by default.
	retainedFrom map[string]uint64
}

func newScenario(t *testing.T) *scenario {
	s1 := fixtureStation("s1", simulatorapi.ReceptionCounters{
		Opportunities: 3, Received: 2, OutsideHorizon: 1,
		ChannelA: simulatorapi.ChannelCounters{Opportunities: 2, Received: 1}, ChannelB: simulatorapi.ChannelCounters{Opportunities: 1, Received: 1},
	})
	s2 := fixtureStation("s2", simulatorapi.ReceptionCounters{
		Opportunities: 3, Received: 2, ProbabilisticLoss: 1,
		ChannelA: simulatorapi.ChannelCounters{Opportunities: 2, Received: 1}, ChannelB: simulatorapi.ChannelCounters{Opportunities: 1, Received: 1},
	})
	tx2 := sentence(t, 200000001, 52.00, ais.ChannelB, start.Add(10*time.Second))
	tx3 := sentence(t, 200000001, 52.01, ais.ChannelA, start.Add(18*time.Second))
	return &scenario{
		stations: []simulatorapi.StationObservation{s1, s2},
		receptions: []simulatorapi.Reception{
			fixtureReception(s2, 1, 1, 200000002, "A", start.Add(5*time.Second), noPositionSentence),
			fixtureReception(s1, 1, 2, 200000001, "B", start.Add(10*time.Second), tx2),
			fixtureReception(s2, 2, 2, 200000001, "B", start.Add(10*time.Second), tx2),
			fixtureReception(s1, 2, 3, 200000001, "A", start.Add(18*time.Second), tx3),
		},
		transmissions: 3,
		retainedFrom:  map[string]uint64{},
	}
}

func (s *scenario) hasStation(id string) bool {
	return slices.ContainsFunc(s.stations, func(st simulatorapi.StationObservation) bool { return st.ID == id })
}

func status(age time.Duration) string {
	switch {
	case age <= 10*time.Second:
		return "fresh"
	case age <= time.Minute:
		return "stale"
	}
	return "lost"
}

// observations derives the snapshot for selection, nil meaning all stations,
// the way the simulator does: each target uses the newest selected reception.
func (s *scenario) observations(selection []string) simulatorapi.Observations {
	if selection == nil {
		for _, st := range s.stations {
			selection = append(selection, st.ID)
		}
	}
	selected := make(map[string]bool)
	for _, id := range selection {
		selected[id] = true
	}
	o := simulatorapi.Observations{
		Metadata: metadata(), StateRevision: 7, StationSetRevision: 2, SnapshotAt: now,
		Selection: slices.Sorted(slices.Values(selection)), Stations: slices.Clone(s.stations),
		Targets: []simulatorapi.ObservedTarget{}, RecentReceptions: []simulatorapi.Reception{}, RecentIsSample: true,
		Transmissions: s.transmissions, Receptions: uint64(len(s.receptions)),
	}
	last := make(map[uint32]map[string]simulatorapi.Reception)
	received := make(map[uint64]bool)
	for _, r := range s.receptions {
		if last[r.MMSI] == nil {
			last[r.MMSI] = make(map[string]simulatorapi.Reception)
		}
		last[r.MMSI][r.StationID] = r
		received[r.TransmissionSequence] = true
		if selected[r.StationID] {
			o.RecentReceptions = append(o.RecentReceptions, r)
		}
	}
	o.ReceivedTransmissions = uint64(len(received))
	for i := range o.Stations {
		if o.Stations[i].Counters.Received > 0 {
			o.Stations[i].OldestReception = new(cmp.Or(s.retainedFrom[o.Stations[i].ID], 1))
			o.Stations[i].LatestReception = new(o.Stations[i].Counters.Received)
		}
	}
	for _, mmsi := range slices.Sorted(maps.Keys(last)) {
		var chosen *simulatorapi.Reception
		var provenance []simulatorapi.TargetStation
		for i, st := range o.Stations {
			r, ok := last[mmsi][st.ID]
			if !ok {
				continue
			}
			age := now.Sub(r.Timestamp)
			if status(age) == "lost" {
				o.Stations[i].LostTargets++
			} else {
				o.Stations[i].CurrentTargets++
			}
			if !selected[st.ID] {
				continue
			}
			if chosen == nil || r.TransmissionSequence > chosen.TransmissionSequence {
				chosen = &r
			}
			provenance = append(provenance, simulatorapi.TargetStation{
				StationID: st.ID, StationEnabled: st.Definition.Enabled, Sequence: r.Sequence, TransmissionSequence: r.TransmissionSequence,
				Timestamp: r.Timestamp, Channel: r.Channel, ReceivedPowerDBm: r.Link.ReceivedPowerDBm, RFRevision: r.RFRevision,
				AgeMs: age.Milliseconds(), Status: status(age),
			})
		}
		if chosen == nil {
			continue
		}
		for i := range provenance {
			provenance[i].Chosen = provenance[i].TransmissionSequence == chosen.TransmissionSequence
		}
		age := now.Sub(chosen.Timestamp)
		o.Targets = append(o.Targets, simulatorapi.ObservedTarget{MMSI: mmsi, Report: *chosen, AgeMs: age.Milliseconds(), Status: status(age), Stations: provenance})
		if status(age) == "lost" {
			o.LostTargets++
		} else {
			o.CurrentTargets++
		}
	}
	slices.SortStableFunc(o.RecentReceptions, func(a, b simulatorapi.Reception) int {
		return cmp.Or(a.Timestamp.Compare(b.Timestamp), cmp.Compare(a.TransmissionSequence, b.TransmissionSequence), cmp.Compare(a.StationID, b.StationID))
	})
	return o
}

// page derives a history page the way the simulator does.
func (s *scenario) page(stationID string, after *uint64, limit int) simulatorapi.ReceptionPage {
	var retained []simulatorapi.Reception
	for _, r := range s.receptions {
		if r.StationID == stationID && r.Sequence >= cmp.Or(s.retainedFrom[stationID], 1) {
			retained = append(retained, r)
		}
	}
	page := simulatorapi.ReceptionPage{SimulationID: "run-1", StationID: stationID, Tail: after == nil, Receptions: []simulatorapi.Reception{}}
	if len(retained) > 0 {
		page.OldestAvailable, page.LatestAvailable = new(retained[0].Sequence), new(retained[len(retained)-1].Sequence)
		if *page.OldestAvailable > 1 {
			page.TruncatedBefore = page.OldestAvailable
		}
	}
	from := max(len(retained)-limit, 0)
	if after != nil {
		page.NextAfter = *after
		from = len(retained)
		for i, r := range retained {
			if r.Sequence > *after {
				from = i
				break
			}
		}
		page.Gap = page.OldestAvailable != nil && *after+1 < *page.OldestAvailable
	}
	to := min(from+limit, len(retained))
	page.Receptions = append(page.Receptions, retained[from:to]...)
	if to > from {
		page.NextAfter = retained[to-1].Sequence
	}
	page.HasMore = to < len(retained)
	return page
}

// upstream is a fixture simulator serving only the observation and reception
// history routes. Any other route access fails the test.
type upstream struct {
	server *httptest.Server

	mu       sync.Mutex
	scenario *scenario
	edit     func(*simulatorapi.Observations)
	// editBody changes the encoded observation body, for invalid JSON forms.
	editBody   func(string) string
	editPage   func(*simulatorapi.ReceptionPage)
	override   http.HandlerFunc // Serves every request when set.
	requests   []string
	unexpected []string
}

func newUpstream(t *testing.T) *upstream {
	u := &upstream{scenario: newScenario(t)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/observations", u.observations)
	mux.HandleFunc("GET /api/stations/{id}/receptions", u.receptions)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.unexpected = append(u.unexpected, r.Method+" "+r.URL.Path)
		u.mu.Unlock()
		http.Error(w, "unexpected route", http.StatusInternalServerError)
	})
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.requests = append(u.requests, r.URL.RequestURI())
		override := u.override
		u.mu.Unlock()
		if override != nil {
			override(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		u.server.Close()
		u.mu.Lock()
		defer u.mu.Unlock()
		require.Empty(t, u.unexpected, "the display reads only observation and reception routes")
	})
	return u
}

func writeFixtureJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func apiError(w http.ResponseWriter, status int, problem simulatorapi.APIError) {
	data, _ := json.Marshal(problem)
	writeFixtureJSON(w, status, data)
}

func (u *upstream) observations(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	defer u.mu.Unlock()
	var selection []string
	if value := r.URL.Query().Get("stations"); value != "all" {
		selection = strings.Split(value, ",")
	}
	for _, id := range selection {
		if !u.scenario.hasStation(id) {
			apiError(w, http.StatusNotFound, simulatorapi.APIError{Error: "station not found: " + id})
			return
		}
	}
	o := u.scenario.observations(selection)
	if u.edit != nil {
		u.edit(&o)
	}
	data, err := json.Marshal(o)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if u.editBody != nil {
		data = []byte(u.editBody(string(data)))
	}
	writeFixtureJSON(w, http.StatusOK, data)
}

func (u *upstream) receptions(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	defer u.mu.Unlock()
	query := r.URL.Query()
	if query.Get("simulationId") != "run-1" {
		apiError(w, http.StatusConflict, simulatorapi.APIError{Error: "simulation identity changed", SimulationID: "run-1"})
		return
	}
	if !u.scenario.hasStation(r.PathValue("id")) {
		apiError(w, http.StatusNotFound, simulatorapi.APIError{Error: "station not found"})
		return
	}
	var after *uint64
	if query.Has("after") {
		n, _ := strconv.ParseUint(query.Get("after"), 10, 64)
		after = &n
	}
	limit := 100
	if query.Has("limit") {
		limit, _ = strconv.Atoi(query.Get("limit"))
	}
	page := u.scenario.page(r.PathValue("id"), after, limit)
	if u.editPage != nil {
		u.editPage(&page)
	}
	data, err := json.Marshal(page)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeFixtureJSON(w, http.StatusOK, data)
}

func (u *upstream) set(change func(*upstream)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	change(u)
}

func (u *upstream) requested() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return slices.Clone(u.requests)
}

func newAPI(t *testing.T, u *upstream) http.Handler {
	t.Helper()
	client, err := display.NewClient(u.server.URL)
	require.NoError(t, err)
	t.Cleanup(client.CloseIdleConnections)
	return display.NewAPI(slog.New(slog.DiscardHandler), client)
}

func get(handler http.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
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

const allObservations = "/display/api/observations?stations=all"

func TestObservationsProjectReceivedReports(t *testing.T) {
	u := newUpstream(t)

	rec := get(newAPI(t, u), allObservations)

	o := decode[display.Observations](t, rec)
	require.Equal(t, "run-1", o.SimulationID)
	require.Equal(t, simulatorapi.TimeState{Now: now, ElapsedMs: 20000, Speed: 1}, o.Time)
	require.Equal(t, []string{"s1", "s2"}, o.Selection)
	require.Len(t, o.Stations, 2)
	require.True(t, o.Stations[0].Selected)
	require.Equal(t, 1, o.Stations[0].CurrentTargets)
	require.Equal(t, []simulatorapi.VesselType{{ID: "cargo", Name: "Cargo vessel"}}, o.ScenarioCategories)
	require.Equal(t, 2, o.CurrentTargets)
	require.Len(t, o.Targets, 2)

	// Two stations with different last receptions: s1 heard tx3, s2 last heard tx2.
	target := o.Targets[0]
	require.Equal(t, uint32(200000001), target.MMSI)
	require.Equal(t, "fresh", target.Status)
	require.Equal(t, int64(2000), target.AgeMs)
	require.Equal(t, "s1", target.Report.StationID)
	require.Equal(t, uint64(3), target.Report.TransmissionSequence)
	require.Equal(t, "A", target.Report.Channel)
	require.Equal(t, 1, target.Report.MessageType)
	require.Equal(t, start.Add(18*time.Second), target.Report.ReceivedAt)
	require.InDelta(t, 52.01, *target.Report.Navigation.Latitude, 1.0/600000)
	require.InDelta(t, 10.5, *target.Report.Navigation.Speed, 1e-9)
	require.Equal(t, 18, *target.Report.Navigation.UTCSecond)
	require.Equal(t, display.Scenario{Name: "Vessel 1", CategoryID: "cargo"}, target.Report.Scenario)
	require.InDelta(t, -80.0, target.Report.Signal.EstimatedPowerDBm, 1e-9)
	require.Equal(t, "Station s1", target.Report.Receiver.Name)
	require.Equal(t, []display.TargetStation{
		{StationID: "s1", StationEnabled: true, Sequence: 2, TransmissionSequence: 3, ReceivedAt: start.Add(18 * time.Second), Channel: "A", EstimatedPowerDBm: -80, RFRevision: 1, CurrentRFRevision: true, AgeMs: 2000, Status: "fresh", Chosen: true},
		{StationID: "s2", StationEnabled: true, Sequence: 2, TransmissionSequence: 2, ReceivedAt: start.Add(10 * time.Second), Channel: "B", EstimatedPowerDBm: -80, RFRevision: 1, CurrentRFRevision: true, AgeMs: 10000, Status: "fresh"},
	}, target.Stations)

	// A target without position stays in the list with null navigation.
	noFix := o.Targets[1]
	require.Equal(t, "stale", noFix.Status)
	require.Nil(t, noFix.Report.Navigation.Latitude)
	require.Nil(t, noFix.Report.Navigation.Longitude)
	require.InDelta(t, 8.0, *noFix.Report.Navigation.Speed, 1e-9)
	require.Contains(t, rec.Body.String(), `"latitude":null,"longitude":null`)

	require.Len(t, o.RecentReceptions, 4)
	require.Equal(t, noPositionSentence, o.RecentReceptions[0].Sentence)
	require.Equal(t, []string{"/api/observations?stations=all"}, u.requested())
}

func TestObservationsSelectionUsesSelectedStationsOnly(t *testing.T) {
	u := newUpstream(t)

	o := decode[display.Observations](t, get(newAPI(t, u), "/display/api/observations?stations=s2"))

	require.Equal(t, []string{"/api/observations?stations=s2"}, u.requested(), "selector is forwarded")
	require.Equal(t, []string{"s2"}, o.Selection)
	require.False(t, o.Stations[0].Selected)
	require.True(t, o.Stations[1].Selected)
	// s2 received the older tx2 and missed tx3: its view keeps the old position.
	target := o.Targets[0]
	require.Equal(t, uint64(2), target.Report.TransmissionSequence)
	require.InDelta(t, 52.00, *target.Report.Navigation.Latitude, 1.0/600000)
	require.Len(t, target.Stations, 1)
	require.True(t, target.Stations[0].Chosen)
	require.Len(t, o.RecentReceptions, 2)
}

func TestObservationsOverlapIsOneTarget(t *testing.T) {
	u := newUpstream(t)
	u.set(func(u *upstream) {
		s := u.scenario
		tx3 := s.receptions[3]
		s.receptions = append(s.receptions, fixtureReception(s.stations[1], 3, 3, tx3.MMSI, tx3.Channel, tx3.Timestamp, tx3.Sentence))
		s.stations[1].Counters.ProbabilisticLoss, s.stations[1].Counters.Received = 0, 3
		s.stations[1].Counters.ChannelA.Received = 2
		s.stations[1].ReceiveRatio, s.stations[1].Recent.ReceiveRatio = new(1.0), new(1.0)
		s.stations[1].Recent.Received, s.stations[1].Recent.ReceptionRate = 3, new(0.15)
	})

	o := decode[display.Observations](t, get(newAPI(t, u), allObservations))

	require.Len(t, o.Targets, 2)
	target := o.Targets[0]
	require.Equal(t, "s1", target.Report.StationID, "the first receiver in station order is reported")
	require.Len(t, target.Stations, 2)
	for _, provenance := range target.Stations {
		require.True(t, provenance.Chosen)
		require.Equal(t, uint64(3), provenance.TransmissionSequence)
	}
	require.Len(t, o.RecentReceptions, 5)
	require.Equal(t, o.RecentReceptions[3].Navigation, o.RecentReceptions[4].Navigation)
}

func TestObservationsWithoutReceptions(t *testing.T) {
	u := newUpstream(t)
	u.set(func(u *upstream) {
		u.scenario.receptions = nil
		for i := range u.scenario.stations {
			u.scenario.stations[i] = fixtureStation(u.scenario.stations[i].ID, simulatorapi.ReceptionCounters{})
		}
	})

	rec := get(newAPI(t, u), allObservations)

	o := decode[display.Observations](t, rec)
	require.Empty(t, o.Targets)
	require.Contains(t, rec.Body.String(), `"targets":[]`)
	require.Contains(t, rec.Body.String(), `"recentReceptions":[]`)
	require.Len(t, o.Stations, 2)
	require.Nil(t, o.Stations[0].ReceiveRatio)
}

func editObservations(change func(*simulatorapi.Observations)) func(*upstream) {
	return func(u *upstream) { u.edit = change }
}

func replaceBody(old, replacement string) func(*upstream) {
	return func(u *upstream) {
		u.editBody = func(body string) string { return strings.Replace(body, old, replacement, 1) }
	}
}

func respond(status int, contentType, body string) func(*upstream) {
	return func(u *upstream) {
		u.override = func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}
	}
}

// badChecksum replaces the two checksum digits of an NMEA sentence.
func badChecksum(s string) string {
	i := strings.LastIndex(s, "*")
	digits := "00"
	if s[i+1:i+3] == digits {
		digits = "01"
	}
	return s[:i+1] + digits + "\r\n"
}

func TestObservationsRejectInvalidUpstream(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*upstream)
		wantErr string
	}{
		{"wrong content type", respond(http.StatusOK, "text/plain", "{}"), "content type"},
		{"invalid JSON", respond(http.StatusOK, "application/json", "{"), "invalid simulator response"},
		{"data after object", respond(http.StatusOK, "application/json", "{} {}"), "decode body"},
		{"unexpected status", respond(http.StatusInternalServerError, "application/json", `{"error":"boom"}`), "unexpected status"},
		{"error without API error", respond(http.StatusNotFound, "application/json", `{}`), "without an API error"},
		{"oversized body", respond(http.StatusOK, "application/json", `{"x":"`+strings.Repeat("x", simulatorapi.ObservationResponseLimit)+`"}`), "exceeds"},
		{"revision overflow", replaceBody(`"stateRevision":"7"`, `"stateRevision":"18446744073709551616"`), "stateRevision"},
		{"revision leading zero", replaceBody(`"stateRevision":"7"`, `"stateRevision":"07"`), "noncanonical"},
		{"negative revision", replaceBody(`"stateRevision":"7"`, `"stateRevision":"-7"`), "invalid unsigned decimal"},
		{"numeric revision", replaceBody(`"stateRevision":"7"`, `"stateRevision":7`), "decode body"},
		{"missing time.now", replaceBody(`"now":"2026-09-14T12:00:20Z",`, ""), "time.now"},
		{"speed between steps", editObservations(func(o *simulatorapi.Observations) { o.Time.Speed = 0.015 }), "time.speed"},
		{"unsupported message type", editObservations(func(o *simulatorapi.Observations) { o.SupportedMessageTypes = []int{5} }), "type 1"},
		{"invalid observation limits", editObservations(func(o *simulatorapi.Observations) { o.Settings.Observation.StaleAgeMs = 5000 }), "observation settings"},
		{"missing revision", editObservations(func(o *simulatorapi.Observations) { o.StateRevision = 0 }), "stateRevision"},
		{"snapshot differs from clock", editObservations(func(o *simulatorapi.Observations) { o.SnapshotAt = now.Add(-time.Second) }), "snapshotAt"},
		{"missing targets", editObservations(func(o *simulatorapi.Observations) { o.Targets = nil }), "required"},
		{"too many targets", editObservations(func(o *simulatorapi.Observations) { o.Settings.Observation.TargetLimit = 1 }), "targetLimit"},
		{"selection mismatch", editObservations(func(o *simulatorapi.Observations) { o.Selection = []string{"s1"} }), "selection"},
		{"duplicate station", editObservations(func(o *simulatorapi.Observations) { o.Stations[1].ID = "s1" }), "duplicate"},
		{"invalid station location", editObservations(func(o *simulatorapi.Observations) { o.Stations[0].Definition.Latitude = 86 }), "location"},
		{"counters do not partition", editObservations(func(o *simulatorapi.Observations) { o.Stations[0].Counters.OutsideHorizon = 2 }), "sum to opportunities"},
		{"impossible bounds", editObservations(func(o *simulatorapi.Observations) { o.Stations[0].LatestReception = new(uint64(5)) }), "bounds"},
		{"geometry type", editObservations(func(o *simulatorapi.Observations) { o.Stations[0].Coverage[0].Geometry.Type = "Polygon" }), "MultiPolygon"},
		{"open ring", editObservations(func(o *simulatorapi.Observations) {
			o.Stations[0].Coverage[0].Geometry.Coordinates[0][0][4] = [2]float64{4, 52}
		}), "closed"},
		{"coordinate range", editObservations(func(o *simulatorapi.Observations) {
			o.Stations[0].Coverage[1].Geometry.Coordinates[0][0][1] = [2]float64{4.1, 91}
		}), "WGS84"},
		{"missing contour", editObservations(func(o *simulatorapi.Observations) { o.Stations[0].Coverage = o.Stations[0].Coverage[:3] }), "contours"},
		{"bad checksum", editObservations(func(o *simulatorapi.Observations) {
			o.Targets[0].Report.Sentence = badChecksum(o.Targets[0].Report.Sentence)
		}), "checksum"},
		{"payload MMSI mismatch", editObservations(func(o *simulatorapi.Observations) { o.RecentReceptions[0].MMSI = 200000009 }), "payload MMSI"},
		{"sentence channel mismatch", editObservations(func(o *simulatorapi.Observations) { o.RecentReceptions[0].Channel = "B" }), "sentence channel"},
		{"unsupported channel", editObservations(func(o *simulatorapi.Observations) { o.RecentReceptions[0].Channel = "C" }), "not A or B"},
		{"future reception", editObservations(func(o *simulatorapi.Observations) { o.RecentReceptions[3].Timestamp = now.Add(time.Second) }), "outside"},
		{"payload second mismatch", editObservations(func(o *simulatorapi.Observations) { o.RecentReceptions[0].Timestamp = start.Add(6 * time.Second) }), "UTC second"},
		{"invalid probability", editObservations(func(o *simulatorapi.Observations) { o.RecentReceptions[0].Link.Probability = 1.5 }), "probability"},
		{"unknown report station", editObservations(func(o *simulatorapi.Observations) { o.Targets[0].Report.StationID = "s9" }), "unknown station"},
		{"unknown provenance station", editObservations(func(o *simulatorapi.Observations) { o.Targets[1].Stations[0].StationID = "s9" }), "unknown station"},
		{"unknown recent station", editObservations(func(o *simulatorapi.Observations) { o.RecentReceptions[0].StationID = "s9" }), "unknown station"},
		{"reception beyond run", editObservations(func(o *simulatorapi.Observations) { o.Transmissions, o.ReceivedTransmissions = 2, 2 }), "beyond"},
		{"duplicate target", editObservations(func(o *simulatorapi.Observations) { o.Targets = append(o.Targets, o.Targets[1]) }), "duplicate target"},
		{"duplicate recent reception", editObservations(func(o *simulatorapi.Observations) {
			o.RecentReceptions = append(o.RecentReceptions, o.RecentReceptions[3])
		}), "duplicate recent"},
		{"status disagrees with age", editObservations(func(o *simulatorapi.Observations) { o.Targets[0].Status = "stale" }), "status"},
		{"age disagrees with clock", editObservations(func(o *simulatorapi.Observations) { o.Targets[0].AgeMs = 1 }), "ageMs"},
		{"wrong chosen flag", editObservations(func(o *simulatorapi.Observations) { o.Targets[0].Stations[1].Chosen = true }), "chosen"},
		{"report station missing from provenance", editObservations(func(o *simulatorapi.Observations) { o.Targets[0].Stations = o.Targets[0].Stations[1:] }), "missing from provenance"},
		{"transmission references disagree", editObservations(func(o *simulatorapi.Observations) {
			o.RecentReceptions[3].Sentence = sentenceFor(o.RecentReceptions[3], 52.5)
		}), "transmission 3 disagree"},
		{"reception names two transmissions", editObservations(func(o *simulatorapi.Observations) { o.RecentReceptions[3].TransmissionSequence = 2 }), "refers to transmissions"},
		{"target counts disagree", editObservations(func(o *simulatorapi.Observations) { o.CurrentTargets = 3 }), "currentTargets"},
		{"station target counts disagree", editObservations(func(o *simulatorapi.Observations) { o.Stations[0].CurrentTargets = 0 }), "target counts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := newUpstream(t)
			handler := newAPI(t, u)
			u.set(tt.change)

			rec := get(handler, allObservations)

			require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), tt.wantErr)
			require.NotContains(t, rec.Body.String(), `"targets"`)
		})
	}
}

// sentenceFor encodes another valid report with the MMSI, channel, and UTC
// second of r at latitude.
func sentenceFor(r simulatorapi.Reception, latitude float64) string {
	s, err := ais.EncodePosition(ais.Position{MMSI: r.MMSI, Latitude: latitude, Longitude: 3.95, Speed: 10.5, Course: 45, Heading: 45, UpdatedAt: r.Timestamp}, ais.Channel(r.Channel))
	if err != nil {
		panic(err)
	}
	return s
}

func TestObservationsRequestErrors(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		change       func(*upstream)
		wantStatus   int
		wantBody     string
		wantUpstream bool
	}{
		{name: "unreachable", target: allObservations, change: func(u *upstream) { u.server.Close() }, wantStatus: http.StatusServiceUnavailable, wantBody: "simulator unavailable"},
		{name: "unknown station", target: "/display/api/observations?stations=s1,s9", wantStatus: http.StatusNotFound, wantBody: "station not found: s9", wantUpstream: true},
		{name: "default selection", target: "/display/api/observations", wantStatus: http.StatusOK, wantUpstream: true},
		{name: "empty selector", target: "/display/api/observations?stations=", wantStatus: http.StatusBadRequest},
		{name: "empty station ID", target: "/display/api/observations?stations=s1,", wantStatus: http.StatusBadRequest},
		{name: "duplicate station", target: "/display/api/observations?stations=s1,s1", wantStatus: http.StatusBadRequest},
		{name: "all mixed with IDs", target: "/display/api/observations?stations=all,s1", wantStatus: http.StatusBadRequest},
		{name: "unknown parameter", target: "/display/api/observations?station=s1", wantStatus: http.StatusBadRequest},
		{name: "repeated parameter", target: "/display/api/observations?stations=s1&stations=s2", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := newUpstream(t)
			handler := newAPI(t, u)
			if tt.change != nil {
				u.set(tt.change)
			}

			rec := get(handler, tt.target)

			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), tt.wantBody)
			if tt.wantStatus != http.StatusOK {
				require.NotContains(t, rec.Body.String(), `"targets"`)
			}
			if tt.change == nil {
				require.Equal(t, tt.wantUpstream, len(u.requested()) == 1)
			}
		})
	}
}

func TestObservationsConcurrentReaders(t *testing.T) {
	u := newUpstream(t)
	handler := newAPI(t, u)
	want := get(handler, allObservations).Body.String()

	bodies := make(chan string, 8)
	var wg sync.WaitGroup
	for range cap(bodies) {
		wg.Go(func() { bodies <- get(handler, allObservations).Body.String() })
	}
	wg.Wait()
	close(bodies)

	for body := range bodies {
		require.Equal(t, want, body)
	}
}

func TestObservationsCancellationStopsUpstreamRead(t *testing.T) {
	u := newUpstream(t)
	handler := newAPI(t, u)
	started, stopped := make(chan struct{}), make(chan struct{})
	u.set(func(u *upstream) {
		u.override = func(_ http.ResponseWriter, r *http.Request) {
			close(started)
			<-r.Context().Done()
			close(stopped)
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, allObservations, nil))
		done <- rec
	}()

	<-started
	cancel()

	<-stopped
	require.Equal(t, http.StatusServiceUnavailable, (<-done).Code)
}

func TestReceptionHistoryProjectsPages(t *testing.T) {
	u := newUpstream(t)
	handler := newAPI(t, u)

	tail := decode[display.ReceptionPage](t, get(handler, "/display/api/stations/s1/receptions?simulationId=run-1"))
	require.True(t, tail.Tail)
	require.Len(t, tail.Receptions, 2)
	require.InDelta(t, 52.00, *tail.Receptions[0].Navigation.Latitude, 1.0/600000)
	require.InDelta(t, 52.01, *tail.Receptions[1].Navigation.Latitude, 1.0/600000)
	require.Equal(t, "B", tail.Receptions[0].Channel)
	require.Equal(t, uint64(2), tail.NextAfter)
	require.False(t, tail.HasMore)
	require.False(t, tail.Gap)
	require.Nil(t, tail.TruncatedBefore)

	next := decode[display.ReceptionPage](t, get(handler, "/display/api/stations/s1/receptions?simulationId=run-1&after=1&limit=1"))
	require.False(t, next.Tail)
	require.Len(t, next.Receptions, 1)
	require.Equal(t, uint64(2), next.Receptions[0].Sequence)

	empty := decode[display.ReceptionPage](t, get(handler, "/display/api/stations/s1/receptions?simulationId=run-1&after=2"))
	require.Empty(t, empty.Receptions)
	require.Equal(t, uint64(2), empty.NextAfter)

	u.set(func(u *upstream) { u.scenario.retainedFrom["s1"] = 2 })
	gap := decode[display.ReceptionPage](t, get(handler, "/display/api/stations/s1/receptions?simulationId=run-1&after=0"))
	require.True(t, gap.Gap)
	require.Equal(t, uint64(2), *gap.TruncatedBefore)
	require.Len(t, gap.Receptions, 1)

	require.Equal(t, []string{
		"/api/stations/s1/receptions?simulationId=run-1",
		"/api/stations/s1/receptions?after=1&limit=1&simulationId=run-1",
		"/api/stations/s1/receptions?after=2&simulationId=run-1",
		"/api/stations/s1/receptions?after=0&simulationId=run-1",
	}, u.requested())
}

func TestReceptionHistoryErrors(t *testing.T) {
	const path = "/display/api/stations/s1/receptions"
	tests := []struct {
		name       string
		target     string
		change     func(*upstream)
		wantStatus int
		wantBody   string
	}{
		{name: "run conflict", target: path + "?simulationId=run-0", wantStatus: http.StatusConflict, wantBody: "simulation identity changed"},
		{name: "unknown station", target: "/display/api/stations/s9/receptions?simulationId=run-1", wantStatus: http.StatusNotFound, wantBody: "station not found"},
		{name: "unreachable", target: path + "?simulationId=run-1", change: func(u *upstream) { u.server.Close() }, wantStatus: http.StatusServiceUnavailable},
		{name: "missing simulation ID", target: path, wantStatus: http.StatusBadRequest},
		{name: "noncanonical cursor", target: path + "?simulationId=run-1&after=01", wantStatus: http.StatusBadRequest},
		{name: "negative cursor", target: path + "?simulationId=run-1&after=-1", wantStatus: http.StatusBadRequest},
		{name: "cursor overflow", target: path + "?simulationId=run-1&after=18446744073709551616", wantStatus: http.StatusBadRequest},
		{name: "zero limit", target: path + "?simulationId=run-1&limit=0", wantStatus: http.StatusBadRequest},
		{name: "limit above maximum", target: path + "?simulationId=run-1&limit=201", wantStatus: http.StatusBadRequest},
		{name: "unknown parameter", target: path + "?simulationId=run-1&cursor=1", wantStatus: http.StatusBadRequest},
		{name: "reordered receptions", target: path + "?simulationId=run-1", change: func(u *upstream) {
			u.editPage = func(p *simulatorapi.ReceptionPage) { slices.Reverse(p.Receptions) }
		}, wantStatus: http.StatusBadGateway, wantBody: "does not follow"},
		{name: "wrong next cursor", target: path + "?simulationId=run-1", change: func(u *upstream) {
			u.editPage = func(p *simulatorapi.ReceptionPage) { p.NextAfter = 1 }
		}, wantStatus: http.StatusBadGateway, wantBody: "nextAfter"},
		{name: "wrong gap", target: path + "?simulationId=run-1&after=0", change: func(u *upstream) {
			u.editPage = func(p *simulatorapi.ReceptionPage) { p.Gap = true }
		}, wantStatus: http.StatusBadGateway, wantBody: "gap"},
		{name: "other station", target: path + "?simulationId=run-1", change: func(u *upstream) {
			u.editPage = func(p *simulatorapi.ReceptionPage) { p.Receptions[0].StationID = "s2" }
		}, wantStatus: http.StatusBadGateway, wantBody: "belongs to station"},
		{name: "bad checksum", target: path + "?simulationId=run-1", change: func(u *upstream) {
			u.editPage = func(p *simulatorapi.ReceptionPage) { p.Receptions[1].Sentence = badChecksum(p.Receptions[1].Sentence) }
		}, wantStatus: http.StatusBadGateway, wantBody: "checksum"},
		{name: "oversized body", target: path + "?simulationId=run-1", change: respond(http.StatusOK, "application/json", `{"x":"`+strings.Repeat("x", simulatorapi.ReceptionResponseLimit)+`"}`), wantStatus: http.StatusBadGateway, wantBody: "exceeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := newUpstream(t)
			handler := newAPI(t, u)
			if tt.change != nil {
				u.set(tt.change)
			}

			rec := get(handler, tt.target)

			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), tt.wantBody)
			require.NotContains(t, rec.Body.String(), `"receptions"`)
			if tt.wantStatus == http.StatusBadRequest {
				require.Empty(t, u.requested(), "invalid queries are not forwarded")
			}
		})
	}
}

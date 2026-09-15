package simulator

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/ais"
	"github.com/miroslav-matejovsky/ais-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/ais-testbench/simulation"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

func stationDefinition() simulatorapi.StationDefinition {
	c := simulatorapi.ReceiverChannel{Enabled: true, SensitivityDBm: -125}
	return simulatorapi.StationDefinition{
		Name: "Receiver", Latitude: 52.02, Longitude: 3.97, Enabled: true,
		AntennaHeightMeters: 40, ReceiveGainDBi: 20, ChannelA: c, ChannelB: c,
		ShadowSectors: []simulatorapi.ShadowSector{},
	}
}

func stationBody(t *testing.T, revision uint64, definition simulatorapi.StationDefinition) string {
	t.Helper()
	d, err := json.Marshal(definition)
	require.NoError(t, err)
	body, err := json.Marshal(simulatorapi.StationRequest{SimulationID: "run-1", StationSetRevision: fmt.Sprint(revision), Definition: d})
	require.NoError(t, err)
	return string(body)
}

func addSite(t *testing.T, f fixture, revision uint64) simulatorapi.StationCreated {
	t.Helper()
	rec := serve(f.handler, "POST", "/api/stations", stationBody(t, revision, stationDefinition()))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	var result simulatorapi.StationCreated
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	require.Equal(t, "stations/"+result.StationID, rec.Header().Get("Location"), "relative to the request URL")
	return result
}

func TestStationHTTPCommandsAndObservations(t *testing.T) {
	f := newFixture(t)
	f.clock.Add(time.Second)
	created := addSite(t, f, 1)
	require.Equal(t, uint64(2), created.StationSetRevision)
	require.Equal(t, start.Add(time.Second), created.SnapshotAt)
	require.Equal(t, created.SnapshotAt, created.Time.Now)
	require.Equal(t, created.SnapshotAt, created.Stations[0].CreatedAt)
	empty := decode[simulatorapi.Observations](t, serve(f.handler, "GET", "/api/observations", ""))
	require.Empty(t, empty.Targets, "station must not receive earlier fleet reports retroactively")
	require.True(t, empty.RecentIsSample)
	f.clock.Add(time.Second)
	decode[simulatorapi.Metadata](t, serve(f.handler, "PUT", "/api/time", `{"speed":1}`))
	o := decode[simulatorapi.Observations](t, serve(f.handler, "GET", "/api/observations?stations="+created.StationID, ""))
	require.Len(t, o.Targets, 1)
	require.Equal(t, f.sim.Fleet().Vessels[0].Report.Sentence, o.Targets[0].Report.Sentence)
	require.Equal(t, f.sim.Fleet().Vessels[0].Report.Sequence, o.Targets[0].Report.TransmissionSequence)
	require.Equal(t, o.Time.Now, o.Targets[0].Report.Timestamp)
	require.Equal(t, uint64(1), o.Stations[0].Counters.Opportunities)
	lastReceived := o.Targets[0].Report
	d := created.Stations[0].Definition
	d.Enabled = false
	updated := decode[simulatorapi.StationSet](t, serve(f.handler, "PUT", "/api/stations/"+created.StationID, stationBody(t, 2, d)))
	require.Equal(t, uint64(3), updated.StationSetRevision)
	require.Equal(t, uint64(2), updated.Stations[0].RFRevision)
	for _, c := range updated.Stations[0].Coverage {
		require.Empty(t, c.Geometry.Coordinates)
	}
	f.clock.Add(time.Second)
	decode[simulatorapi.Metadata](t, serve(f.handler, "PUT", "/api/time", `{"speed":1}`))
	o = decode[simulatorapi.Observations](t, serve(f.handler, "GET", "/api/observations", ""))
	require.Equal(t, lastReceived, o.Targets[0].Report, "missed transmission cannot update target navigation")
	require.Equal(t, int64(1000), o.Targets[0].AgeMs)
	require.Equal(t, uint64(1), o.Stations[0].Counters.StationDisabled)
	// No-op edit leaves the station revision, then a name-only edit keeps RF revision.
	noOp := decode[simulatorapi.StationSet](t, serve(f.handler, "PUT", "/api/stations/"+created.StationID, stationBody(t, 3, d)))
	require.Equal(t, uint64(3), noOp.StationSetRevision)
	d.Name = "Renamed"
	renamed := decode[simulatorapi.StationSet](t, serve(f.handler, "PUT", "/api/stations/"+created.StationID, stationBody(t, 3, d)))
	require.Equal(t, uint64(2), renamed.Stations[0].RFRevision)
	page := decode[simulatorapi.ReceptionPage](t, serve(f.handler, "GET", "/api/stations/"+created.StationID+"/receptions?simulationId=run-1", ""))
	require.Equal(t, "Receiver", page.Receptions[0].Receiver.Name, "renaming must preserve reception-time attribution")
	removed := decode[simulatorapi.StationSet](t, serve(f.handler, "DELETE", "/api/stations/"+created.StationID+"?simulationId=run-1&stationSetRevision=4", ""))
	require.Empty(t, removed.Stations)
	require.NotEmpty(t, f.sim.Fleet().Vessels)
	require.Equal(t, 404, serve(f.handler, "GET", "/api/stations/"+created.StationID+"/receptions?simulationId=run-1", "").Code)
	added := addSite(t, f, 5)
	require.NotEqual(t, created.StationID, added.StationID)
}

func TestStationReadsNeverSettleAndRejectStaleWrites(t *testing.T) {
	f := newFixture(t)
	created := addSite(t, f, 1)
	f.clock.Add(10 * time.Second)
	before := f.sim.StationConfiguration()
	for _, path := range []string{"/api/stations", "/api/observations", "/api/stations/" + created.StationID + "/receptions?simulationId=run-1"} {
		require.Equal(t, 200, serve(f.handler, "GET", path, "").Code)
	}
	for _, body := range []string{stationBody(t, 1, stationDefinition()), strings.Replace(stationBody(t, 2, stationDefinition()), "run-1", "previous-run", 1)} {
		rec := serve(f.handler, "PUT", "/api/stations/"+created.StationID, body)
		require.Equal(t, 409, rec.Code)
		var conflict simulatorapi.APIError
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &conflict))
		require.Equal(t, "run-1", conflict.SimulationID)
		require.Equal(t, uint64(2), *conflict.StationSetRevision)
	}
	require.Equal(t, before, f.sim.StationConfiguration())
}

func TestStationRequestsValidateBeforeSettlement(t *testing.T) {
	valid := stationBody(t, 1, stationDefinition())
	for _, tt := range []struct {
		name, body string
		status     int
	}{
		{"null", "null", 400}, {"array", "[]", 400}, {"second object", valid + " {}", 400},
		{"missing run", strings.Replace(valid, `"simulationId":"run-1",`, "", 1), 400},
		{"numeric revision", strings.Replace(valid, `"stationSetRevision":"1"`, `"stationSetRevision":1`, 1), 400},
		{"zero revision", strings.Replace(valid, `"stationSetRevision":"1"`, `"stationSetRevision":"0"`, 1), 400},
		{"missing enabled", strings.Replace(valid, `"enabled":true,`, "", 1), 400},
		{"missing zero", strings.Replace(valid, `"feederLossDb":0,`, "", 1), 400},
		{"null channel", strings.Replace(valid, `"channelA":{`, `"channelA":null,"ignored":{`, 1), 400},
		{"missing sensitivity", strings.Replace(valid, `"sensitivityDbm":-125,`, "", 1), 400},
		{"null sectors", strings.Replace(valid, `"shadowSectors":[]`, `"shadowSectors":null`, 1), 400},
		{"bad sector", strings.Replace(valid, `"shadowSectors":[]`, `"shadowSectors":[{"startDegrees":0,"endDegrees":30}]`, 1), 400},
		{"unknown field", strings.Replace(valid, `"name":"Receiver"`, `"name":"Receiver","surprise":1`, 1), 400},
		{"nonfinite", strings.Replace(valid, `"latitude":52.02`, `"latitude":1e999`, 1), 400},
		{"height", strings.Replace(valid, `"antennaHeightMeters":40`, `"antennaHeightMeters":0`, 1), 400},
		{"oversized", valid + strings.Repeat(" ", simulatorapi.StationRequestLimit), 413},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.clock.Add(time.Second)
			before := f.sim.StationConfiguration()
			rec := serve(f.handler, "POST", "/api/stations", tt.body)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			require.Equal(t, before, f.sim.StationConfiguration())
		})
	}
	f := newFixture(t)
	req := httptest.NewRequest("POST", "/api/stations", strings.NewReader(valid))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	require.Equal(t, 415, rec.Code)
	rec = serve(f.handler, "PATCH", "/api/stations", valid)
	require.Equal(t, 405, rec.Code)
	require.Contains(t, rec.Header().Get("Allow"), "POST")
	// False and zero are legitimate required values.
	d := stationDefinition()
	d.Enabled = false
	d.ChannelA.Enabled = false
	d.ChannelB.Enabled = false
	require.Equal(t, 201, serve(f.handler, "POST", "/api/stations", stationBody(t, 1, d)).Code)
}

func TestStationSelectorsAndHistoryQueries(t *testing.T) {
	f := newFixture(t)
	first := addSite(t, f, 1)
	second := addSite(t, f, 2)
	selected := decode[simulatorapi.Observations](t, serve(f.handler, "GET", "/api/observations?stations="+second.StationID+","+first.StationID, ""))
	require.Equal(t, []string{first.StationID, second.StationID}, selected.Selection)
	for _, tt := range []struct {
		path   string
		status int
	}{
		{"/api/observations?stations=", 400}, {"/api/observations?stations=station-1,station-1", 400},
		{"/api/observations?stations=all,station-1", 400}, {"/api/observations?stations=unknown", 404},
		{"/api/observations?stations=all&stations=all", 400}, {"/api/stations?unexpected=1", 400},
		{"/api/stations/station-1/receptions", 400},
		{"/api/stations/station-1/receptions?simulationId=old-run", 409},
		{"/api/stations/missing/receptions?simulationId=run-1", 404},
		{"/api/stations/station-1/receptions?simulationId=run-1&after=1", 400},
		{"/api/stations/station-1/receptions?simulationId=run-1&after=18446744073709551616", 400},
		{"/api/stations/station-1/receptions?simulationId=run-1&after=01", 400},
		{"/api/stations/station-1/receptions?simulationId=run-1&after=-1", 400},
		{"/api/stations/station-1/receptions?simulationId=run-1&after=", 400},
		{"/api/stations/station-1/receptions?simulationId=run-1&limit=0", 400},
		{"/api/stations/station-1/receptions?simulationId=run-1&limit=201", 400},
	} {
		t.Run(tt.path, func(t *testing.T) {
			rec := serve(f.handler, "GET", tt.path, "")
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
		})
	}
	page := decode[simulatorapi.ReceptionPage](t, serve(f.handler, "GET", "/api/stations/station-1/receptions?simulationId=run-1&after=0", ""))
	require.Nil(t, page.OldestAvailable)
	require.Nil(t, page.LatestAvailable)
	require.Empty(t, page.Receptions)
	require.Zero(t, page.NextAfter)
	require.False(t, page.Gap)
}

func TestReceptionHistoryRollover(t *testing.T) {
	f := newFixture(t)
	addSite(t, f, 1)
	_, err := f.sim.SetCount(100)
	require.NoError(t, err)
	_, err = f.sim.Advance(t.Context(), 12*time.Second)
	require.NoError(t, err)
	path := "/api/stations/station-1/receptions?simulationId=run-1"
	tail := decode[simulatorapi.ReceptionPage](t, serve(f.handler, "GET", path, ""))
	require.True(t, tail.Tail)
	require.Len(t, tail.Receptions, 100)
	require.NotNil(t, tail.TruncatedBefore)
	require.False(t, tail.Gap)
	page := decode[simulatorapi.ReceptionPage](t, serve(f.handler, "GET", path+"&after=0&limit=200", ""))
	require.True(t, page.Gap)
	require.True(t, page.HasMore)
	require.Len(t, page.Receptions, 200)
	require.Equal(t, *page.OldestAvailable, page.Receptions[0].Sequence)
	for _, r := range page.Receptions {
		decoded, err := ais.DecodePosition(r.Sentence)
		require.NoError(t, err)
		require.Equal(t, r.MMSI, decoded.MMSI)
	}
	next := decode[simulatorapi.ReceptionPage](t, serve(f.handler, "GET", path+fmt.Sprintf("&after=%d", page.NextAfter), ""))
	require.False(t, next.Gap)
	require.Equal(t, page.NextAfter+1, next.Receptions[0].Sequence)
	none := decode[simulatorapi.ReceptionPage](t, serve(f.handler, "GET", path+fmt.Sprintf("&after=%d", *tail.LatestAvailable), ""))
	require.Empty(t, none.Receptions)
	require.Equal(t, *tail.LatestAvailable, none.NextAfter)
	o := decode[simulatorapi.Observations](t, serve(f.handler, "GET", "/api/observations", ""))
	require.Len(t, o.Targets, 100, "history eviction must not erase live target state")
}

func TestStationConcurrentEditsReturnOwnSnapshot(t *testing.T) {
	f := newFixture(t)
	created := addSite(t, f, 1)
	f.clock.Add(time.Second)
	results := make(chan *httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"First", "Second"} {
		d := stationDefinition()
		d.Name = name
		body := stationBody(t, 2, d)
		wg.Go(func() { results <- serve(f.handler, "PUT", "/api/stations/"+created.StationID, body) })
	}
	wg.Wait()
	close(results)
	statuses := make(map[int]int)
	for rec := range results {
		statuses[rec.Code]++
		if rec.Code == 200 {
			set := decode[simulatorapi.StationSet](t, rec)
			require.Equal(t, uint64(3), set.StationSetRevision)
			require.Equal(t, start.Add(time.Second), set.SnapshotAt)
		}
	}
	require.Equal(t, map[int]int{200: 1, 409: 1}, statuses)
}

func TestStationRuntimeDefaultsReceiveInitialReports(t *testing.T) {
	config := simdriver.NewConfig()
	config.Speed = 0
	sim, err := simulation.New(config)
	require.NoError(t, err)
	handler := newAPI(slog.New(slog.DiscardHandler), &Simulator{driver: simdriver.NewDriver(sim, &testClock{now: start})})
	o := decode[simulatorapi.Observations](t, serve(handler, "GET", "/observations", ""))
	require.Len(t, o.Stations, 3)
	require.Equal(t, uint64(1), o.Transmissions)
	for _, s := range o.Stations {
		require.Equal(t, uint64(1), s.Counters.Opportunities)
	}
	require.NotEmpty(t, o.Targets)
}

func TestStationFieldErrorsAndApplicationFailure(t *testing.T) {
	f := newFixture(t)
	d := stationDefinition()
	d.ChannelA.SensitivityDBm = -10
	d.AntennaHeightMeters = 0
	rec := serve(f.handler, "POST", "/api/stations", stationBody(t, 1, d))
	require.Equal(t, 400, rec.Code)
	var problem simulatorapi.APIError
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	require.Contains(t, problem.Fields, "channelA.sensitivityDbm")
	require.Contains(t, problem.Fields, "antennaHeightMeters")
	f.clock.Add(simdriver.MaxCatchUp + time.Second)
	before := f.sim.StationConfiguration()
	rec = serve(f.handler, "POST", "/api/stations", stationBody(t, 1, stationDefinition()))
	require.Equal(t, 500, rec.Code)
	require.Equal(t, before, f.sim.StationConfiguration())
	for _, method := range []string{"PUT", "DELETE"} {
		path, body := "/api/stations/unknown", stationBody(t, 1, stationDefinition())
		if method == "DELETE" {
			path += "?simulationId=run-1&stationSetRevision=1"
			body = ""
		}
		require.Equal(t, 404, serve(f.handler, method, path, body).Code)
	}
}

func TestObservationHTTPReadsStayCoherentDuringTicks(t *testing.T) {
	f := newFixture(t)
	addSite(t, f, 1)
	done := make(chan error, 1)
	go func() {
		for range 30 {
			f.clock.Add(time.Second)
			if err := f.driver.SetSpeed(t.Context(), 1); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for range 30 {
		o := decode[simulatorapi.Observations](t, serve(f.handler, "GET", "/api/observations", ""))
		require.Equal(t, o.Time.Now, o.SnapshotAt)
		for _, target := range o.Targets {
			require.False(t, target.Report.Timestamp.After(o.SnapshotAt))
			require.Equal(t, o.SnapshotAt.Sub(target.Report.Timestamp).Milliseconds(), target.AgeMs)
		}
	}
	require.NoError(t, <-done)
}

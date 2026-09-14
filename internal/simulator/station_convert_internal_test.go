package simulator

import (
	"encoding/json"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

func TestCoverageGeoJSON(t *testing.T) {
	for _, tt := range []struct {
		name   string
		ring   []simulation.GeoPoint
		pieces int
	}{
		{"empty", nil, 0},
		{"ordinary", []simulation.GeoPoint{{Latitude: 1, Longitude: 1}, {Latitude: 1, Longitude: 2}, {Latitude: 0, Longitude: 2}, {Latitude: 0, Longitude: 1}, {Latitude: 1, Longitude: 1}}, 1},
		{"east crossing", []simulation.GeoPoint{{Latitude: 1, Longitude: 179}, {Latitude: 1, Longitude: 181}, {Latitude: 0, Longitude: 181}, {Latitude: 0, Longitude: 179}, {Latitude: 1, Longitude: 179}}, 2},
		{"west crossing", []simulation.GeoPoint{{Latitude: 85, Longitude: -181}, {Latitude: 85, Longitude: -179}, {Latitude: 84, Longitude: -179}, {Latitude: 84, Longitude: -181}, {Latitude: 85, Longitude: -181}}, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original := slices.Clone(tt.ring)
			g := coverageGeometry(tt.ring)
			require.Equal(t, "MultiPolygon", g.Type)
			require.Len(t, g.Coordinates, tt.pieces)
			for _, polygon := range g.Coordinates {
				ring := polygon[0]
				require.Equal(t, ring[0], ring[len(ring)-1])
				area := 0.0
				for i := 0; i < len(ring)-1; i++ {
					p, q := ring[i], ring[i+1]
					require.GreaterOrEqual(t, p[0], -180.0)
					require.LessOrEqual(t, p[0], 180.0)
					require.LessOrEqual(t, math.Abs(p[0]-q[0]), 180.0)
					area += p[0]*q[1] - q[0]*p[1]
				}
				require.Positive(t, area)
			}
			require.Equal(t, original, tt.ring)
		})
	}
}

func TestReceptionIdentityPreservesUint64(t *testing.T) {
	r := simulation.Reception{Sequence: math.MaxUint64, TransmissionSequence: math.MaxUint64 - 1, ConfigRevision: math.MaxUint64, RFRevision: math.MaxUint64,
		StationID: "station-1", Channel: simulation.ChannelB, Timestamp: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), Sentence: "exact bytes\r\n"}
	wire := receptionResponse(r)
	data, err := json.Marshal(wire)
	require.NoError(t, err)
	require.Contains(t, string(data), `"sequence":"18446744073709551615"`)
	require.Contains(t, string(data), `"transmissionSequence":"18446744073709551614"`)
	var decoded simulatorapi.Reception
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, wire, decoded)
}

func TestMaximumObservationAndHistoryPayloads(t *testing.T) {
	c := simulation.ReceiverChannel{Enabled: true, SensitivityDBm: -125, NoisePenaltyDB: 0.0000000000000001}
	d := simulation.StationDefinition{Name: strings.Repeat("\x01", 80), Latitude: 52.02, Longitude: 3.97, Enabled: true,
		AntennaHeightMeters: 500, ReceiveGainDBi: 20, ChannelA: c, ChannelB: c}
	for i := 0; i < 8; i++ {
		d.ShadowSectors = append(d.ShadowSectors, simulation.ShadowSector{StartDegrees: float64(i * 40), EndDegrees: float64(i*40 + 20), LossDB: 0.0000000000000001})
	}
	config := simulation.Config{ID: "payload-test", StartTime: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), Seed: 42, InitialVesselCount: 100, Speed: 1,
		Transmitter: simulation.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1}}
	for range simulation.MaxStations {
		config.Stations = append(config.Stations, d)
	}
	sim, err := simulation.New(config)
	require.NoError(t, err)
	for range 9 {
		_, err = sim.SetCount(0)
		require.NoError(t, err)
		_, err = sim.SetCount(100)
		require.NoError(t, err)
	}
	o, err := sim.Observations(nil)
	require.NoError(t, err)
	require.Len(t, o.Targets, 1000)
	wire := observationsResponse(o)
	for _, target := range wire.Targets {
		require.Len(t, target.Stations, 16)
	}
	body, err := json.Marshal(wire)
	require.NoError(t, err)
	require.Less(t, len(body), simulatorapi.ObservationResponseLimit)
	page, err := sim.ReceptionHistory("station-1", nil, 200)
	require.NoError(t, err)
	pageBody, err := json.Marshal(receptionPageResponse(page))
	require.NoError(t, err)
	require.Less(t, len(pageBody), simulatorapi.ReceptionResponseLimit)
	definitionBody, err := json.Marshal(wire.Stations[0].Definition)
	require.NoError(t, err)
	require.Less(t, len(definitionBody), simulatorapi.StationDefinitionLimit)
	t.Logf("maximum occupancy: observations=%d bytes, 200 receptions=%d bytes, definition=%d bytes", len(body), len(pageBody), len(definitionBody))
}

func TestEmptyStationWireCollections(t *testing.T) {
	body, err := json.Marshal(stationSetResponse(simulation.StationConfiguration{}))
	require.NoError(t, err)
	require.Contains(t, string(body), `"stations":[]`)
	body, err = json.Marshal(observationsResponse(simulation.Observations{}))
	require.NoError(t, err)
	for _, field := range []string{"selection", "stations", "targets", "recentReceptions"} {
		require.Contains(t, string(body), `"`+field+`":[]`)
	}
	body, err = json.Marshal(receptionPageResponse(simulation.ReceptionPage{}))
	require.NoError(t, err)
	for _, field := range []string{"oldestAvailable", "latestAvailable", "truncatedBefore"} {
		require.Contains(t, string(body), `"`+field+`":null`)
	}
}

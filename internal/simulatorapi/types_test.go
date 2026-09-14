package simulatorapi_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
)

const sentence = "!AIVDM,1,1,,A,13u?etPv2;0n:dDPwUM1U1Cb069D,0*24\r\n"

func TestFleetJSON(t *testing.T) {
	at := time.Date(2026, 9, 14, 12, 0, 0, 500, time.UTC)
	fleet := simulatorapi.Fleet{
		SimulationID: "run-1", UpdatedAt: at, MessageCount: 1, MessageLimit: 1000,
		Vessels: []simulatorapi.Vessel{{
			MMSI: 200000000, Name: "Vessel 1", TypeID: "cargo",
			Report: simulatorapi.Report{Sequence: 1, Timestamp: at, Sentence: sentence},
		}},
	}
	want := `{"simulationId":"run-1","updatedAt":"2026-09-14T12:00:00.0000005Z","messageCount":1,"messageLimit":1000,` +
		`"vessels":[{"mmsi":200000000,"name":"Vessel 1","typeId":"cargo",` +
		`"report":{"sequence":1,"timestamp":"2026-09-14T12:00:00.0000005Z","sentence":"!AIVDM,1,1,,A,13u?etPv2;0n:dDPwUM1U1Cb069D,0*24\r\n"}}]}`
	requireRoundTrip(t, fleet, want)

	empty := simulatorapi.Fleet{SimulationID: "run-1", UpdatedAt: at, MessageLimit: 1000, Vessels: []simulatorapi.Vessel{}}
	requireRoundTrip(t, empty, `{"simulationId":"run-1","updatedAt":"2026-09-14T12:00:00.0000005Z","messageCount":0,"messageLimit":1000,"vessels":[]}`)
}

func TestHistoryJSON(t *testing.T) {
	at := time.Date(2026, 9, 14, 12, 0, 1, 0, time.UTC)
	oldest, latest := uint64(4), uint64(5)
	history := simulatorapi.History{
		SimulationID: "run-1", MessageLimit: 1000, OldestSequence: &oldest, LatestSequence: &latest,
		Messages: []simulatorapi.Message{
			{Sequence: 4, MMSI: 200000000, Timestamp: at, Sentence: sentence},
			{Sequence: 5, MMSI: 200000001, Timestamp: at, Sentence: sentence},
		},
	}
	message := `"timestamp":"2026-09-14T12:00:01Z","sentence":"!AIVDM,1,1,,A,13u?etPv2;0n:dDPwUM1U1Cb069D,0*24\r\n"}`
	requireRoundTrip(t, history, `{"simulationId":"run-1","messageLimit":1000,"oldestSequence":4,"latestSequence":5,"messages":[`+
		`{"sequence":4,"mmsi":200000000,`+message+`,{"sequence":5,"mmsi":200000001,`+message+`]}`)

	empty := simulatorapi.History{SimulationID: "run-1", MessageLimit: 1000, Messages: []simulatorapi.Message{}}
	requireRoundTrip(t, empty, `{"simulationId":"run-1","messageLimit":1000,"oldestSequence":null,"latestSequence":null,"messages":[]}`)
}

func TestMetadataJSON(t *testing.T) {
	startedAt := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	metadata := simulatorapi.Metadata{
		SimulationID: "opaque-run-identity",
		StartedAt:    startedAt,
		Time: simulatorapi.TimeState{
			Now: startedAt.Add(5*time.Second + 999), ElapsedMs: 5000, Speed: 0, Paused: true,
		},
		VesselTypes:           []simulatorapi.VesselType{{ID: "cargo", Name: "Cargo vessel"}},
		SupportedMessageTypes: []int{1},
		Settings: simulatorapi.Settings{
			InitialVesselCount: 1, MaxVessels: 100, TickIntervalMs: 1000, MessageIntervalMs: 1000, PacingIntervalMs: 100, MessageHistoryLimit: 1000,
			SpeedKnots:  simulatorapi.SpeedRange{Min: 6, Max: 15.9},
			Speed:       simulatorapi.SpeedLimits{Min: 0.01, Max: 100, Step: 0.01},
			SpawnBounds: simulatorapi.SpawnBounds{South: 52, North: 52.04, West: 3.94, East: 4},
		},
	}
	requireRoundTrip(t, metadata, `{
		"simulationId": "opaque-run-identity",
		"startedAt": "2026-09-14T12:00:00Z",
		"time": { "now": "2026-09-14T12:00:05.000000999Z", "elapsedMs": 5000, "speed": 0, "paused": true },
		"vesselTypes": [{ "id": "cargo", "name": "Cargo vessel" }],
		"supportedMessageTypes": [1],
		"settings": {
			"initialVesselCount": 1,
			"maxVessels": 100,
			"tickIntervalMs": 1000,
			"messageIntervalMs": 1000,
			"pacingIntervalMs": 100,
			"messageHistoryLimit": 1000,
			"speedKnots": { "min": 6, "max": 15.9 },
			"speed": { "min": 0.01, "max": 100, "step": 0.01 },
			"spawnBounds": { "south": 52, "north": 52.04, "west": 3.94, "east": 4 }
		}
	}`)
}

func TestCountRequestJSON(t *testing.T) {
	var request simulatorapi.CountRequest
	require.NoError(t, json.Unmarshal([]byte(`{"count":0}`), &request))
	require.NotNil(t, request.Count)
	require.Equal(t, 0, *request.Count)

	request = simulatorapi.CountRequest{}
	require.NoError(t, json.Unmarshal([]byte(`{"count":null}`), &request))
	require.Nil(t, request.Count)
}

func TestTimeRequestJSON(t *testing.T) {
	var request simulatorapi.TimeRequest
	require.NoError(t, json.Unmarshal([]byte(`{"speed":0}`), &request))
	require.NotNil(t, request.Speed, "0 is a valid pause speed")
	require.InDelta(t, 0.0, *request.Speed, 0)

	request = simulatorapi.TimeRequest{}
	require.NoError(t, json.Unmarshal([]byte(`{"speed":null}`), &request))
	require.Nil(t, request.Speed)
}

// requireRoundTrip checks the exact field names and values in want, and that
// decoding restores value, including CRLF and nanosecond UTC timestamps.
func requireRoundTrip[T any](t *testing.T, value T, want string) {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	require.JSONEq(t, want, string(data))
	var decoded T
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, value, decoded)
}

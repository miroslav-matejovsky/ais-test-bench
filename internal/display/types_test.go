package display_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/display"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
)

func TestFleetJSON(t *testing.T) {
	at := time.Date(2026, 9, 14, 12, 0, 5, 0, time.UTC)
	bounds := simulatorapi.SpawnBounds{South: 52, North: 52.04, West: 3.94, East: 4}

	tests := []struct {
		name  string
		fleet display.Fleet
		want  string
	}{
		{
			name:  "empty fleet",
			fleet: display.Fleet{SimulationID: "run-1", UpdatedAt: at, SpawnBounds: bounds, Vessels: []display.Vessel{}},
			want:  `{"simulationId":"run-1","updatedAt":"2026-09-14T12:00:05Z","spawnBounds":{"south":52,"north":52.04,"west":3.94,"east":4},"vessels":[]}`,
		},
		{
			name: "available navigation",
			fleet: display.Fleet{SimulationID: "run-1", UpdatedAt: at, SpawnBounds: bounds, Vessels: []display.Vessel{{
				MMSI: 200000001, Name: "Vessel 1", TypeID: "cargo", TypeName: "Cargo vessel",
				Latitude: new(52.01), Longitude: new(3.95), Speed: new(10.5), Course: new(45.0), Heading: new(45), UpdatedAt: at,
			}}},
			want: `{"simulationId":"run-1","updatedAt":"2026-09-14T12:00:05Z","spawnBounds":{"south":52,"north":52.04,"west":3.94,"east":4},"vessels":[
				{"mmsi":200000001,"name":"Vessel 1","typeId":"cargo","typeName":"Cargo vessel","latitude":52.01,"longitude":3.95,"speed":10.5,"course":45,"heading":45,"updatedAt":"2026-09-14T12:00:05Z"}]}`,
		},
		{
			name: "unavailable navigation is null",
			fleet: display.Fleet{SimulationID: "run-1", UpdatedAt: at, SpawnBounds: bounds, Vessels: []display.Vessel{{
				MMSI: 200000002, Name: "Vessel 2", TypeID: "cargo", TypeName: "Cargo vessel", UpdatedAt: at,
			}}},
			want: `{"simulationId":"run-1","updatedAt":"2026-09-14T12:00:05Z","spawnBounds":{"south":52,"north":52.04,"west":3.94,"east":4},"vessels":[
				{"mmsi":200000002,"name":"Vessel 2","typeId":"cargo","typeName":"Cargo vessel","latitude":null,"longitude":null,"speed":null,"course":null,"heading":null,"updatedAt":"2026-09-14T12:00:05Z"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.fleet)
			require.NoError(t, err)
			require.JSONEq(t, tt.want, string(data))

			var decoded display.Fleet
			require.NoError(t, json.Unmarshal(data, &decoded))
			require.Equal(t, tt.fleet, decoded)
		})
	}
}

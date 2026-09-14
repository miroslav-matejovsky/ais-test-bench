package simulator

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

func TestResponsesPreserveEngineValues(t *testing.T) {
	at := time.Date(2030, 1, 2, 3, 4, 5, 6, time.UTC)
	sentence := "!AIVDM,1,1,,A,12vg200P190B7O2MhS2dRb2B0000,0*06\r\n"
	oldest, latest := uint64(1), uint64(2)

	fleet := fleetResponse(simulation.Fleet{
		SimulationID: "run-1", UpdatedAt: at, MessageCount: 2, MessageLimit: 1000,
		Vessels: []simulation.Vessel{{
			MMSI: 200000000, Name: "Vessel 1", TypeID: "cargo",
			Report: simulation.Report{Sequence: 2, Timestamp: at, Sentence: sentence},
		}},
	})
	require.Equal(t, simulatorapi.Fleet{
		SimulationID: "run-1", UpdatedAt: at, MessageCount: 2, MessageLimit: 1000,
		Vessels: []simulatorapi.Vessel{{
			MMSI: 200000000, Name: "Vessel 1", TypeID: "cargo",
			Report: simulatorapi.Report{Sequence: 2, Timestamp: at, Sentence: sentence},
		}},
	}, fleet)

	history := historyResponse(simulation.History{
		SimulationID: "run-1", MessageLimit: 1000, OldestSequence: &oldest, LatestSequence: &latest,
		Messages: []simulation.Message{
			{Sequence: 1, MMSI: 200000000, Timestamp: at, Sentence: sentence},
			{Sequence: 2, MMSI: 200000001, Timestamp: at.Add(time.Second), Sentence: sentence},
		},
	})
	require.Equal(t, simulatorapi.History{
		SimulationID: "run-1", MessageLimit: 1000, OldestSequence: &oldest, LatestSequence: &latest,
		Messages: []simulatorapi.Message{
			{Sequence: 1, MMSI: 200000000, Timestamp: at, Sentence: sentence},
			{Sequence: 2, MMSI: 200000001, Timestamp: at.Add(time.Second), Sentence: sentence},
		},
	}, history)
}

func TestEmptyResponsesEncodeArraysAndNullBounds(t *testing.T) {
	for _, tt := range []struct {
		value    any
		contains []string
	}{
		{fleetResponse(simulation.Fleet{}), []string{`"vessels":[]`}},
		{historyResponse(simulation.History{}), []string{`"oldestSequence":null`, `"latestSequence":null`, `"messages":[]`}},
		{metadataResponse(simulation.Metadata{}), []string{`"vesselTypes":[]`, `"supportedMessageTypes":[]`}},
	} {
		data, err := json.Marshal(tt.value)
		require.NoError(t, err)
		for _, s := range tt.contains {
			require.Contains(t, string(data), s)
		}
	}
}

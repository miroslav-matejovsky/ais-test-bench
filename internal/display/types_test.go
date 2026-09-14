package display_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/display"
	"github.com/miroslav-matejovsky/ais-testbench/internal/simulatorapi"
)

func TestTargetJSON(t *testing.T) {
	at := time.Date(2026, 9, 14, 12, 0, 5, 0, time.UTC)
	channel := simulatorapi.ReceiverChannel{Enabled: true, SensitivityDBm: -110}
	target := display.Target{
		MMSI: 200000002, AgeMs: 1500, Status: "fresh",
		Report: display.Reception{
			StationID: "s1", Sequence: 18446744073709551615, TransmissionSequence: 9, MMSI: 200000002, MessageType: 1,
			Channel: "B", ReceivedAt: at, Sentence: "!AIVDM\r\n",
			Navigation:     display.Navigation{Speed: new(8.0), UTCSecond: new(5)},
			Scenario:       display.Scenario{Name: "Vessel 2", CategoryID: "cargo"},
			ConfigRevision: 2, RFRevision: 1,
			Receiver: simulatorapi.ReceiverSnapshot{Name: "Coast", Latitude: 52, Longitude: 4, AntennaHeightMeters: 30, Channel: channel},
			Signal:   display.Signal{EstimatedPowerDBm: -80.5, EffectiveSensitivityDBm: -110, MarginDB: 29.5, Probability: 1, DistanceMeters: 5000},
		},
		Stations: []display.TargetStation{{
			StationID: "s1", StationEnabled: true, Sequence: 18446744073709551615, TransmissionSequence: 9, ReceivedAt: at,
			Channel: "B", EstimatedPowerDBm: -80.5, RFRevision: 1, CurrentRFRevision: true, AgeMs: 1500, Status: "fresh", Chosen: true,
		}},
	}
	const want = `{"mmsi":200000002,"ageMs":1500,"status":"fresh",
		"report":{"stationId":"s1","sequence":"18446744073709551615","transmissionSequence":"9","mmsi":200000002,"messageType":1,
			"channel":"B","receivedAt":"2026-09-14T12:00:05Z","sentence":"!AIVDM\r\n",
			"navigation":{"latitude":null,"longitude":null,"speed":8,"course":null,"heading":null,"utcSecond":5},
			"scenario":{"name":"Vessel 2","categoryId":"cargo"},"configRevision":"2","rfRevision":"1",
			"receiver":{"name":"Coast","latitude":52,"longitude":4,"antennaHeightMeters":30,"receiveGainDbi":0,"feederLossDb":0,
				"channel":{"enabled":true,"sensitivityDbm":-110,"noisePenaltyDb":0,"dropProbability":0}},
			"signal":{"estimatedPowerDbm":-80.5,"effectiveSensitivityDbm":-110,"marginDb":29.5,"probability":1,
				"distanceMeters":5000,"bearingDegrees":null,"horizonMeters":0,"shadowLossDb":0}},
		"stations":[{"stationId":"s1","stationEnabled":true,"sequence":"18446744073709551615","transmissionSequence":"9",
			"receivedAt":"2026-09-14T12:00:05Z","channel":"B","estimatedPowerDbm":-80.5,"rfRevision":"1","currentRfRevision":true,
			"ageMs":1500,"status":"fresh","chosen":true}]}`

	data, err := json.Marshal(target)
	require.NoError(t, err)
	require.JSONEq(t, want, string(data))

	var decoded display.Target
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, target, decoded)
}

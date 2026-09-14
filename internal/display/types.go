package display

import (
	"time"

	"github.com/miroslav-matejovsky/ais-testbench/internal/simulatorapi"
)

// Observations is the GET /display/api/observations response: one validated
// projection of one atomic simulator observation snapshot. Every target and
// navigation value comes from AIS reports the selected stations received.
// Revisions, sequences, and counters are decimal JSON strings, as upstream.
type Observations struct {
	// SimulationID identifies the simulator run; a change means a restart.
	SimulationID string    `json:"simulationId"`
	StartedAt    time.Time `json:"startedAt"`
	// Time is the committed virtual clock of the snapshot. Every timestamp in
	// the response is at or before Time.Now.
	Time               simulatorapi.TimeState `json:"time"`
	StateRevision      uint64                 `json:"stateRevision,string"`
	StationSetRevision uint64                 `json:"stationSetRevision,string"`
	Settings           Settings               `json:"settings"`
	// ScenarioCategories are scenario vessel categories, not AIS ship types.
	// A category a reception references but the simulator catalogue lacks is
	// appended with its ID as its name.
	ScenarioCategories []simulatorapi.VesselType `json:"scenarioCategories"`
	// Selection is the canonical ascending set of selected station IDs.
	Selection []string `json:"selection"`
	// Stations holds every configured station, selected or not, in ID order.
	Stations []Station `json:"stations"`
	// Targets holds one entry per observed MMSI in the selection, in MMSI order.
	Targets        []Target `json:"targets"`
	CurrentTargets int      `json:"currentTargets"`
	LostTargets    int      `json:"lostTargets"`
	// RecentReceptions is the newest sample of selected receptions, ascending by
	// receive time, transmission sequence, and station ID. It is not a feed.
	RecentReceptions      []Reception `json:"recentReceptions"`
	RecentIsSample        bool        `json:"recentIsSample"`
	Transmissions         uint64      `json:"transmissions,string"`
	ReceivedTransmissions uint64      `json:"receivedTransmissions,string"`
	Receptions            uint64      `json:"receptions,string"`
	TargetEvictions       uint64      `json:"targetEvictions,string"`
}

// Settings are the simulator settings a presentation needs: the reference
// transmitter and reception model that coverage and signal estimates assume,
// observation age and retention limits, and the scenario spawn area.
type Settings struct {
	MaxStations int                              `json:"maxStations"`
	Transmitter simulatorapi.TransmitterProfile  `json:"transmitter"`
	Reception   simulatorapi.ReceptionModel      `json:"reception"`
	Observation simulatorapi.ObservationSettings `json:"observation"`
	SpawnBounds simulatorapi.SpawnBounds         `json:"spawnBounds"`
}

// Station is a configured station with its current configuration, coverage,
// counters, and history bounds exactly as the simulator published them.
// Coverage geometry is simulator-owned estimated presentation data.
type Station struct {
	simulatorapi.StationObservation
	// Selected reports whether the station contributes to Targets.
	Selected bool `json:"selected"`
}

// Target is one observed MMSI. Report is the newest selected reception, the
// only source of its navigation.
type Target struct {
	MMSI uint32 `json:"mmsi"`
	// AgeMs is the virtual time from Report.ReceivedAt to Observations.Time.Now.
	AgeMs int64 `json:"ageMs"`
	// Status is fresh, stale, or lost.
	Status string    `json:"status"`
	Report Reception `json:"report"`
	// Stations is the last reception of every selected station observing the
	// target, in station ID order.
	Stations []TargetStation `json:"stations"`
}

// TargetStation is compact last-seen provenance of one station for a target.
type TargetStation struct {
	StationID            string    `json:"stationId"`
	StationEnabled       bool      `json:"stationEnabled"`
	Sequence             uint64    `json:"sequence,string"`
	TransmissionSequence uint64    `json:"transmissionSequence,string"`
	ReceivedAt           time.Time `json:"receivedAt"`
	Channel              string    `json:"channel"`
	// EstimatedPowerDBm is the model's estimated received power, not a measurement.
	EstimatedPowerDBm float64 `json:"estimatedPowerDbm"`
	RFRevision        uint64  `json:"rfRevision,string"`
	// CurrentRFRevision is false when the reception used an earlier station RF
	// configuration than the station has now.
	CurrentRFRevision bool   `json:"currentRfRevision"`
	AgeMs             int64  `json:"ageMs"`
	Status            string `json:"status"`
	// Chosen is true when this station received the target's chosen transmission.
	Chosen bool `json:"chosen"`
}

// Reception is one successful station reception: the raw NMEA, its decoded
// navigation, scenario labels, and the reception-time station configuration
// and signal estimate. Later station edits never change these values.
type Reception struct {
	StationID            string `json:"stationId"`
	Sequence             uint64 `json:"sequence,string"`
	TransmissionSequence uint64 `json:"transmissionSequence,string"`
	MMSI                 uint32 `json:"mmsi"`
	// MessageType is the AIS message type decoded from Sentence.
	MessageType int    `json:"messageType"`
	Channel     string `json:"channel"`
	// ReceivedAt is the full virtual UTC receive time, equal to the transmission time.
	ReceivedAt time.Time `json:"receivedAt"`
	// Sentence is the exact received checksummed NMEA line including CRLF.
	Sentence   string     `json:"sentence"`
	Navigation Navigation `json:"navigation"`
	Scenario   Scenario   `json:"scenario"`
	// ConfigRevision and RFRevision are the station revisions at reception.
	ConfigRevision uint64 `json:"configRevision,string"`
	RFRevision     uint64 `json:"rfRevision,string"`
	// Receiver is the immutable reception-time station configuration.
	Receiver simulatorapi.ReceiverSnapshot `json:"receiver"`
	Signal   Signal                        `json:"signal"`
}

// Navigation is decoded only from a received AIS sentence. A nil value, JSON
// null, means the report marks it unavailable. Coordinates are decimal
// degrees, speed is knots, course and heading are degrees.
type Navigation struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Speed     *float64 `json:"speed"`
	Course    *float64 `json:"course"`
	Heading   *int     `json:"heading"`
	// UTCSecond is the payload UTC second, 0-59.
	UTCSecond *int `json:"utcSecond"`
}

// Scenario holds simulator scenario labels. Type 1 AIS does not carry them.
// CategoryID refers to Observations.ScenarioCategories.
type Scenario struct {
	Name       string `json:"name"`
	CategoryID string `json:"categoryId"`
}

// Signal is the simulator's estimated link at reception, not an AIS measurement.
type Signal struct {
	EstimatedPowerDBm       float64  `json:"estimatedPowerDbm"`
	EffectiveSensitivityDBm float64  `json:"effectiveSensitivityDbm"`
	MarginDB                float64  `json:"marginDb"`
	Probability             float64  `json:"probability"`
	DistanceMeters          float64  `json:"distanceMeters"`
	BearingDegrees          *float64 `json:"bearingDegrees"`
	HorizonMeters           float64  `json:"horizonMeters"`
	ShadowLossDB            float64  `json:"shadowLossDb"`
}

// ReceptionPage is the GET /display/api/stations/{id}/receptions response: a
// validated, decoded page of one station's ascending reception history.
type ReceptionPage struct {
	SimulationID string `json:"simulationId"`
	StationID    string `json:"stationId"`
	// Tail is true for a request without a cursor: the newest receptions.
	Tail            bool        `json:"tail"`
	OldestAvailable *uint64     `json:"oldestAvailable,string"`
	LatestAvailable *uint64     `json:"latestAvailable,string"`
	TruncatedBefore *uint64     `json:"truncatedBefore,string"`
	Receptions      []Reception `json:"receptions"`
	NextAfter       uint64      `json:"nextAfter,string"`
	HasMore         bool        `json:"hasMore"`
	// Gap is true when receptions after the request cursor were evicted.
	Gap bool `json:"gap"`
}

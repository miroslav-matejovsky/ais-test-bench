package simulatorapi

import "time"

// Fleet is the complete active fleet copied from one consistent engine state.
type Fleet struct {
	// SimulationID is the opaque, nonempty identity of the engine run.
	SimulationID string `json:"simulationId"`
	// UpdatedAt is the UTC time of the latest tick or effective count change.
	UpdatedAt time.Time `json:"updatedAt"`
	// MessageCount is the number of reports currently retained in History.
	MessageCount int `json:"messageCount"`
	// MessageLimit is the history capacity.
	MessageLimit int `json:"messageLimit"`
	// Vessels are ordered by creation. An empty fleet is a non-nil empty slice
	// so it encodes as [].
	Vessels []Vessel `json:"vessels"`
}

// Vessel is one active vessel and its latest AIS report.
type Vessel struct {
	// MMSI is stable within one simulation run.
	MMSI uint32 `json:"mmsi"`
	// Name is a human-readable synthetic name.
	Name string `json:"name"`
	// TypeID references a Metadata.VesselTypes entry.
	TypeID string `json:"typeId"`
	// Report is always present, including immediately after creation.
	Report Report `json:"report"`
}

// Report is one emitted AIS sentence without its vessel identity.
type Report struct {
	// Sequence is the emission number within the simulation run.
	Sequence uint64 `json:"sequence"`
	// Timestamp is the full UTC generation time.
	Timestamp time.Time `json:"timestamp"`
	// Sentence is a complete type 1 !AIVDM sentence including CRLF.
	Sentence string `json:"sentence"`
}

// History is the retained report history, oldest first.
type History struct {
	SimulationID string `json:"simulationId"`
	MessageLimit int    `json:"messageLimit"`
	// OldestSequence and LatestSequence bound the retained messages. Both are
	// nil, encoded as null, when no message is retained.
	OldestSequence *uint64 `json:"oldestSequence"`
	LatestSequence *uint64 `json:"latestSequence"`
	// Messages is a non-nil slice so an empty history encodes as [].
	Messages []Message `json:"messages"`
}

// Message is one retained report with its source vessel. Retained messages of
// removed vessels remain until they are evicted.
type Message struct {
	Sequence  uint64    `json:"sequence"`
	MMSI      uint32    `json:"mmsi"`
	Timestamp time.Time `json:"timestamp"`
	Sentence  string    `json:"sentence"`
}

// Metadata describes one simulation run. It is immutable within a run.
type Metadata struct {
	SimulationID string `json:"simulationId"`
	// StartedAt is the UTC engine start time.
	StartedAt time.Time `json:"startedAt"`
	// VesselTypes lists application categories, not numeric AIS ship types.
	VesselTypes []VesselType `json:"vesselTypes"`
	// SupportedMessageTypes lists the AIS message types the simulator emits.
	SupportedMessageTypes []int    `json:"supportedMessageTypes"`
	Settings              Settings `json:"settings"`
}

// VesselType is one application vessel category.
type VesselType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Settings are the effective generation settings. The live vessel count is
// len(Fleet.Vessels); InitialVesselCount is only the startup count.
type Settings struct {
	InitialVesselCount  int         `json:"initialVesselCount"`
	MaxVessels          int         `json:"maxVessels"`
	TickIntervalMs      int64       `json:"tickIntervalMs"`
	MessageIntervalMs   int64       `json:"messageIntervalMs"`
	MessageHistoryLimit int         `json:"messageHistoryLimit"`
	SpeedKnots          SpeedRange  `json:"speedKnots"`
	SpawnBounds         SpawnBounds `json:"spawnBounds"`
}

// SpeedRange is an inclusive speed range in knots, in 0.1-knot increments.
type SpeedRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// SpawnBounds is the area for new vessels in decimal degrees. South and West
// are inclusive; North and East are exclusive.
type SpawnBounds struct {
	South float64 `json:"south"`
	North float64 `json:"north"`
	West  float64 `json:"west"`
	East  float64 `json:"east"`
}

// CountRequest is the PUT /api/vessels body. Count is a pointer so a missing
// or null count can be rejected.
type CountRequest struct {
	Count *int `json:"count"`
}

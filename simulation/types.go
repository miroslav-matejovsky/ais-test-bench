package simulation

import "time"

// Config is the complete, explicit configuration of one run. Zero Seed,
// InitialVesselCount, and Speed are valid values, not requests for defaults.
type Config struct {
	// ID is the opaque, nonempty run identity.
	ID string
	// StartTime is the initial virtual instant. It is normalized to UTC and must
	// be nonzero and within years 1 through 9999.
	StartTime time.Time
	// Seed determines all random vessel properties.
	Seed uint64
	// InitialVesselCount is the fleet size created by New, 0 through MaxVessels.
	InitialVesselCount int
	// Speed is the initial Elapse multiplier. See Simulator.SetSpeed.
	Speed float64
}

// Fleet is the complete active fleet copied from one consistent engine state.
type Fleet struct {
	// SimulationID is the opaque, nonempty identity of the engine run.
	SimulationID string
	// UpdatedAt is the virtual UTC time of the latest report tick or effective
	// count change. The clock in TimeState.Now can be later.
	UpdatedAt time.Time
	// MessageCount is the number of reports currently retained in History.
	MessageCount int
	// MessageLimit is the history capacity.
	MessageLimit int
	// Vessels are ordered by creation. An empty fleet is a non-nil empty slice.
	Vessels []Vessel
}

// Vessel is one active vessel and its latest AIS report.
type Vessel struct {
	// MMSI is stable within one simulation run.
	MMSI uint32
	// Name is a human-readable synthetic name.
	Name string
	// TypeID references a Metadata.VesselTypes entry.
	TypeID string
	// Report is always present, including immediately after creation.
	Report Report
}

// Report is one emitted AIS sentence without its vessel identity.
type Report struct {
	// Sequence is the emission number within the simulation run, starting at 1.
	Sequence uint64
	// Timestamp is the full virtual UTC generation time. The sentence holds only
	// its UTC second.
	Timestamp time.Time
	// Sentence is a complete checksummed type 1 !AIVDM sentence including CRLF.
	Sentence string
}

// History is the retained report history, oldest first.
type History struct {
	// SimulationID is the identity of the engine run.
	SimulationID string
	// MessageLimit is the history capacity.
	MessageLimit int
	// OldestSequence and LatestSequence bound the retained messages. Both are nil
	// when no message is retained. They point to copies, not engine state.
	OldestSequence *uint64
	LatestSequence *uint64
	// Messages is a non-nil slice, ordered by sequence.
	Messages []Message
}

// Message is one report with its source vessel. Retained messages of removed
// vessels remain until they are evicted.
type Message struct {
	Sequence  uint64
	MMSI      uint32
	Timestamp time.Time // Virtual UTC time; the sentence holds its UTC second.
	Sentence  string
}

// Metadata describes one simulation run, its committed clock, and the settings
// the engine uses. Time changes as the run advances; the rest is fixed per run.
type Metadata struct {
	// SimulationID is the identity of the engine run.
	SimulationID string
	// StartedAt is the initial virtual instant, Config.StartTime in UTC.
	StartedAt time.Time
	// Time is the committed virtual clock and speed.
	Time TimeState
	// VesselTypes lists application categories, not numeric AIS ship types.
	VesselTypes []VesselType
	// SupportedMessageTypes lists the AIS message types the engine emits.
	SupportedMessageTypes []int
	// Settings are the effective generation settings.
	Settings Settings
}

// TimeState is the committed virtual clock.
type TimeState struct {
	// Now is the virtual UTC instant, StartedAt plus Elapsed. It advances between
	// report ticks, so it can be later than the latest report.
	Now time.Time
	// Elapsed is the virtual time since StartedAt.
	Elapsed time.Duration
	// Speed is the normalized Elapse multiplier, a multiple of SpeedStep.
	Speed float64
	// Paused reports Speed 0.
	Paused bool
}

// VesselType is one application vessel category.
type VesselType struct {
	ID   string
	Name string
}

// Settings are the effective generation settings. The live vessel count is
// len(Fleet.Vessels); InitialVesselCount is only the count created by New.
// Intervals are virtual time.
type Settings struct {
	InitialVesselCount  int
	MaxVessels          int
	TickIntervalMs      int64
	MessageIntervalMs   int64
	MessageHistoryLimit int
	SpeedKnots          SpeedRange
	Speed               SpeedLimits
	SpawnBounds         SpawnBounds
}

// SpeedLimits is the accepted running speed range and precision. Speed 0 is the
// separate pause value.
type SpeedLimits struct {
	Min  float64
	Max  float64
	Step float64
}

// SpeedRange is an inclusive speed range in knots, in 0.1-knot increments.
type SpeedRange struct {
	Min float64
	Max float64
}

// SpawnBounds is the area for new vessels in decimal degrees. South and West
// are inclusive; North and East are exclusive.
type SpawnBounds struct {
	South float64
	North float64
	West  float64
	East  float64
}

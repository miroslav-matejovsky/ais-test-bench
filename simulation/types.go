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
	// Transmitter is the run-level reference transmitter every vessel uses. It
	// has no zero-value default; every field must be in its documented range.
	Transmitter TransmitterProfile
	// Stations are the initial receiving stations, 0 through MaxStations. Nil
	// and empty mean zero receivers. New assigns IDs in slice order.
	Stations []StationDefinition
}

// TransmitterProfile is the reference AIS transmitter assumed for all vessels.
// Ranges are simulator limits, not regulatory values.
type TransmitterProfile struct {
	// PowerWatts is the output power, in (0, 25] W.
	PowerWatts float64
	// HeightMeters is the antenna height above mean sea level, in (0, 100] m.
	HeightMeters float64
	// GainDBi is the antenna gain, in [-10, 20] dBi.
	GainDBi float64
	// FeederLossDB is the cable and connector loss, in [0, 30] dB.
	FeederLossDB float64
}

// StationDefinition is the editable configuration of one receiving station.
// All numbers must be finite. A zero value is invalid: both channels need a
// sensitivity and the antenna needs a height.
type StationDefinition struct {
	// Name is a required display label of at most MaxStationNameRunes runes of
	// valid UTF-8, so at most 320 bytes. It is not blank. Duplicates are allowed.
	Name string
	// Latitude is WGS84 decimal degrees in [-85, 85], the web map range, so
	// coverage never encircles a pole.
	Latitude float64
	// Longitude is WGS84 decimal degrees in [-180, 180]; 180 is stored as -180.
	Longitude float64
	// Enabled is the administrative receiving state.
	Enabled bool
	// AntennaHeightMeters is the height above mean sea level, site elevation
	// plus mast, in (0, 500] m.
	AntennaHeightMeters float64
	// ReceiveGainDBi is the single effective antenna gain, in [-10, 20] dBi.
	ReceiveGainDBi float64
	// FeederLossDB is cable, connector, and splitter loss, in [0, 30] dB.
	FeederLossDB float64
	// ChannelA and ChannelB are both required, even when disabled.
	ChannelA ReceiverChannel
	ChannelB ReceiverChannel
	// ShadowSectors are at most MaxShadowSectors non-overlapping bearing
	// intervals with extra loss. Empty means omnidirectional coverage. Stored
	// definitions hold them sorted by StartDegrees.
	ShadowSectors []ShadowSector
}

// ReceiverChannel is the receiving capability of one AIS channel.
type ReceiverChannel struct {
	// Enabled reports whether the station receives this channel. All channels
	// disabled is a valid degraded configuration.
	Enabled bool
	// SensitivityDBm is the reference sensitivity, in [-125, -80] dBm.
	SensitivityDBm float64
	// NoisePenaltyDB degrades the sensitivity, in [0, 40] dB.
	NoisePenaltyDB float64
	// DropProbability is an extra packet-drop probability in [0, 1].
	DropProbability float64
}

// ShadowSector is a bearing interval, clockwise from true north, with extra
// loss. StartDegrees is inclusive and EndDegrees exclusive, both in [0, 360).
// StartDegrees greater than EndDegrees wraps across north. Equal values are
// rejected; a full-circle loss belongs to the common link settings.
type ShadowSector struct {
	StartDegrees float64
	EndDegrees   float64
	// LossDB is the extra loss inside the sector, in [0, 60] dB.
	LossDB float64
}

// Station is one configured receiving station.
type Station struct {
	// ID is the immutable run-local identity, never reused within a run. It is
	// independent of names and positions in the station list.
	ID string
	// Definition is the canonical stored configuration.
	Definition StationDefinition
	// ConfigRevision starts at 1 and increases on every effective edit.
	ConfigRevision uint64
	// RFRevision starts at 1 and increases on every edit that affects reception
	// or coverage, which is every field except Name.
	RFRevision uint64
	// CreatedAt is the virtual UTC instant the station was added.
	CreatedAt time.Time
	// Coverage holds the contours for channel A then B, each at probability 0.9
	// then 0.5, for Settings.Transmitter. It changes only with the RF revision.
	Coverage []Coverage
}

// Channel is an AIS VHF data channel.
type Channel string

// The two AIS channels. Every vessel alternates between them.
const (
	ChannelA Channel = "A" // AIS 1, 161.975 MHz.
	ChannelB Channel = "B" // AIS 2, 162.025 MHz.
)

// Coverage is the estimated area where a transmission from the reference
// transmitter reaches at least Threshold reception probability on one station
// channel. It is a model estimate from the same function that decides
// reception; actual decisions never test ring membership.
type Coverage struct {
	Channel   Channel
	Threshold float64
	// MinRadiusMeters and MaxRadiusMeters bound the sampled radii; both are 0
	// for an empty ring.
	MinRadiusMeters float64
	MaxRadiusMeters float64
	// Ring is a closed WGS84 polygon ring, first point repeated last, sampled
	// every 5 degrees clockwise from north plus both sides of every shadow
	// sector boundary. Longitudes stay continuous around the station, so near
	// the antimeridian they can leave [-180, 180]. It is empty, not nil, when
	// the station, the channel, or the threshold is unreachable.
	Ring []GeoPoint
}

// GeoPoint is a WGS84 position in decimal degrees.
type GeoPoint struct {
	Latitude  float64
	Longitude float64
}

// ReceptionModel lists the fixed parameters and assumptions of the reception
// model. Values are empirical test-bench choices, not calibrated predictions.
type ReceptionModel struct {
	// SiteLossDB is excess loss in dB on every path.
	SiteLossDB float64
	// PathExponent adds 10*(PathExponent-2)*log10(km) dB beyond 1 km.
	PathExponent float64
	// EffectiveEarthRadiusFactor scales the earth radius of the radio horizon.
	EffectiveEarthRadiusFactor float64
	ChannelAFrequencyMHz       float64
	ChannelBFrequencyMHz       float64
	// HorizonTaperStart is the fraction of the horizon where probability starts
	// to fall linearly to 0 at the horizon.
	HorizonTaperStart float64
	// Margin probability interpolates linearly through
	// (ZeroProbabilityMarginDB, 0), (0 dB, ReferenceProbability), and
	// (FullProbabilityMarginDB, 1), clamped outside.
	ZeroProbabilityMarginDB float64
	ReferenceProbability    float64
	FullProbabilityMarginDB float64
	// CoverageThresholds are the probabilities of Station.Coverage contours.
	CoverageThresholds []float64
}

// StationSet is the complete station configuration copied from one consistent
// engine state.
type StationSet struct {
	// SimulationID is the identity of the engine run.
	SimulationID string
	// Revision starts at 1 and increases on every effective add, edit, or
	// removal. Station edits take it as their expected revision.
	Revision uint64
	// Stations are ordered by creation. An empty set is a non-nil empty slice.
	Stations []Station
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
	MaxStations         int
	// Transmitter is Config.Transmitter, the profile every coverage estimate
	// and reception assumes.
	Transmitter TransmitterProfile
	// Reception is the reception model.
	Reception ReceptionModel
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

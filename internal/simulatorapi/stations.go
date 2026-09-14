package simulatorapi

import (
	"encoding/json"
	"time"
)

// Station and observation HTTP resource limits. Byte bounds apply before compression.
const (
	StationRequestLimit      = 64 << 10
	StationDefinitionLimit   = 4 << 10
	ObservationResponseLimit = 8 << 20
	ReceptionResponseLimit   = 2 << 20
	ReceptionPageLimit       = 200
	MaxStationNameRunes      = 80
	MaxShadowSectors         = 8
)

// StationSet is one atomic configuration snapshot, including its clock/settings.
type StationSet struct {
	Metadata
	StateRevision      uint64    `json:"stateRevision,string"`
	StationSetRevision uint64    `json:"stationSetRevision,string"`
	SnapshotAt         time.Time `json:"snapshotAt"`
	Stations           []Station `json:"stations"`
}

// Station is a configured site. Coverage uses Settings.Transmitter and Reception.
type Station struct {
	ID             string            `json:"id"`
	Definition     StationDefinition `json:"definition"`
	ConfigRevision uint64            `json:"configRevision,string"`
	RFRevision     uint64            `json:"rfRevision,string"`
	CreatedAt      time.Time         `json:"createdAt"`
	RFUpdatedAt    time.Time         `json:"rfUpdatedAt"`
	Coverage       []Coverage        `json:"coverage"`
}

// Coverage is a probability contour at the station's current RF revision.
type Coverage struct {
	Channel         string   `json:"channel"`
	Threshold       float64  `json:"threshold"`
	MinRadiusMeters float64  `json:"minRadiusMeters"`
	MaxRadiusMeters float64  `json:"maxRadiusMeters"`
	Geometry        Geometry `json:"geometry"`
}

// Geometry is a GeoJSON MultiPolygon. Coordinates are polygons, rings, then
// [longitude, latitude] pairs in WGS84. Empty coverage is coordinates: [].
// Rings are closed and cut at the antimeridian, with counterclockwise exteriors.
type Geometry struct {
	Type        string           `json:"type"`
	Coordinates [][][][2]float64 `json:"coordinates"`
}

// StationRequest requires the current simulationId and stationSetRevision and a
// complete definition. Definition stays raw until required fields are validated.
type StationRequest struct {
	SimulationID       string          `json:"simulationId"`
	StationSetRevision string          `json:"stationSetRevision"`
	Definition         json.RawMessage `json:"definition"`
}

// StationCreated includes the assigned ID and the exact resulting configuration.
type StationCreated struct {
	StationSet
	StationID string `json:"stationId"`
}

// APIError is an error on a station/observation route. Conflicts include current
// run identity and revision; fields maps invalid input paths to explanations.
type APIError struct {
	Error              string            `json:"error"`
	Fields             map[string]string `json:"fields,omitempty"`
	SimulationID       string            `json:"simulationId,omitempty"`
	StationSetRevision *uint64           `json:"stationSetRevision,string,omitempty"`
}

// Observations is one atomic received-traffic snapshot. No navigation is supplied
// outside received NMEA. RecentReceptions is a tail sample, not a continuous feed.
type Observations struct {
	Metadata
	StateRevision         uint64               `json:"stateRevision,string"`
	StationSetRevision    uint64               `json:"stationSetRevision,string"`
	SnapshotAt            time.Time            `json:"snapshotAt"`
	Selection             []string             `json:"selection"`
	Stations              []StationObservation `json:"stations"`
	Targets               []ObservedTarget     `json:"targets"`
	CurrentTargets        int                  `json:"currentTargets"`
	LostTargets           int                  `json:"lostTargets"`
	RecentReceptions      []Reception          `json:"recentReceptions"`
	RecentIsSample        bool                 `json:"recentIsSample"`
	Transmissions         uint64               `json:"transmissions,string"`
	ReceivedTransmissions uint64               `json:"receivedTransmissions,string"`
	Receptions            uint64               `json:"receptions,string"`
	TargetEvictions       uint64               `json:"targetEvictions,string"`
}

// StationObservation includes all lifetime and recent counters for a site,
// regardless of the target selection. CurrentTargets includes fresh and stale.
type StationObservation struct {
	Station
	Counters        ReceptionCounters `json:"counters"`
	ReceiveRatio    *float64          `json:"receiveRatio"`
	Recent          RecentCounters    `json:"recent"`
	OldestReception *uint64           `json:"oldestReception,string"`
	LatestReception *uint64           `json:"latestReception,string"`
	CurrentTargets  int               `json:"currentTargets"`
	LostTargets     int               `json:"lostTargets"`
}

// RecentCounters describes one-second buckets in the most recent virtual window.
// DurationMs is min(60000, age of station). Rates are per virtual second, null
// when duration is zero; ReceiveRatio is null without opportunities.
type RecentCounters struct {
	DurationMs      int64    `json:"durationMs"`
	Opportunities   uint64   `json:"opportunities,string"`
	Received        uint64   `json:"received,string"`
	OpportunityRate *float64 `json:"opportunityRate"`
	ReceptionRate   *float64 `json:"receptionRate"`
	ReceiveRatio    *float64 `json:"receiveRatio"`
}

// Reception preserves exact received bytes and reception-time RF provenance.
// Timestamp is both transmission and reception UTC; no latency is modelled.
// VesselName/TypeID are scenario labels, not data received in the AIS payload.
type Reception struct {
	StationID            string           `json:"stationId"`
	Sequence             uint64           `json:"sequence,string"`
	TransmissionSequence uint64           `json:"transmissionSequence,string"`
	MMSI                 uint32           `json:"mmsi"`
	Channel              string           `json:"channel"`
	Timestamp            time.Time        `json:"timestamp"`
	Sentence             string           `json:"sentence"`
	VesselName           string           `json:"vesselName"`
	VesselTypeID         string           `json:"vesselTypeId"`
	ConfigRevision       uint64           `json:"configRevision,string"`
	RFRevision           uint64           `json:"rfRevision,string"`
	Receiver             ReceiverSnapshot `json:"receiver"`
	Link                 ReceptionLink    `json:"link"`
}

// ObservedTarget uses the newest report received by any selected station.
// AgeMs is virtual time since that report; status is fresh, stale, or lost.
type ObservedTarget struct {
	MMSI     uint32          `json:"mmsi"`
	Report   Reception       `json:"report"`
	AgeMs    int64           `json:"ageMs"`
	Status   string          `json:"status"`
	Stations []TargetStation `json:"stations"`
}

// TargetStation is compact last-seen provenance. Chosen is true only if this
// station received the chosen target transmission; no older report is upgraded.
type TargetStation struct {
	StationID            string    `json:"stationId"`
	StationEnabled       bool      `json:"stationEnabled"`
	Sequence             uint64    `json:"sequence,string"`
	TransmissionSequence uint64    `json:"transmissionSequence,string"`
	Timestamp            time.Time `json:"timestamp"`
	Channel              string    `json:"channel"`
	ReceivedPowerDBm     float64   `json:"receivedPowerDbm"`
	RFRevision           uint64    `json:"rfRevision,string"`
	AgeMs                int64     `json:"ageMs"`
	Status               string    `json:"status"`
	Chosen               bool      `json:"chosen"`
}

// ReceptionPage is ascending station history. Tail indicates an initial sample;
// Gap indicates eviction after a requested cursor. TruncatedBefore is the first
// retained sequence when earlier history was evicted, otherwise null.
type ReceptionPage struct {
	SimulationID    string      `json:"simulationId"`
	StationID       string      `json:"stationId"`
	Tail            bool        `json:"tail"`
	OldestAvailable *uint64     `json:"oldestAvailable,string"`
	LatestAvailable *uint64     `json:"latestAvailable,string"`
	TruncatedBefore *uint64     `json:"truncatedBefore,string"`
	Receptions      []Reception `json:"receptions"`
	NextAfter       uint64      `json:"nextAfter,string"`
	HasMore         bool        `json:"hasMore"`
	Gap             bool        `json:"gap"`
}

// TransmitterProfile is the simulator wire representation; units and ranges are documented on its fields.
type TransmitterProfile struct {
	// PowerWatts is the output power, in (0, 25] W.
	PowerWatts float64 `json:"powerWatts"`
	// HeightMeters is the antenna height above mean sea level, in (0, 100] m.
	HeightMeters float64 `json:"heightMeters"`
	// GainDBi is the antenna gain, in [-10, 20] dBi.
	GainDBi float64 `json:"gainDbi"`
	// FeederLossDB is the cable and connector loss, in [0, 30] dB.
	FeederLossDB float64 `json:"feederLossDb"`
}

// StationDefinition is the simulator wire representation; units and ranges are documented on its fields.
type StationDefinition struct {
	// Name is a required display label of at most MaxStationNameRunes runes of
	// valid UTF-8, so at most 320 bytes. It is not blank. Duplicates are allowed.
	Name string `json:"name"`
	// Latitude is WGS84 decimal degrees in [-85, 85], the web map range, so
	// coverage never encircles a pole.
	Latitude float64 `json:"latitude"`
	// Longitude is WGS84 decimal degrees in [-180, 180]; 180 is stored as -180.
	Longitude float64 `json:"longitude"`
	// Enabled is the administrative receiving state.
	Enabled bool `json:"enabled"`
	// AntennaHeightMeters is the height above mean sea level, site elevation
	// plus mast, in (0, 500] m.
	AntennaHeightMeters float64 `json:"antennaHeightMeters"`
	// ReceiveGainDBi is the single effective antenna gain, in [-10, 20] dBi.
	ReceiveGainDBi float64 `json:"receiveGainDbi"`
	// FeederLossDB is cable, connector, and splitter loss, in [0, 30] dB.
	FeederLossDB float64 `json:"feederLossDb"`
	// ChannelA and ChannelB are both required, even when disabled.
	ChannelA ReceiverChannel `json:"channelA"`
	ChannelB ReceiverChannel `json:"channelB"`
	// ShadowSectors are at most MaxShadowSectors non-overlapping bearing
	// intervals with extra loss. Empty means omnidirectional coverage. Stored
	// definitions hold them sorted by StartDegrees.
	ShadowSectors []ShadowSector `json:"shadowSectors"`
}

// ReceiverChannel is the simulator wire representation; units and ranges are documented on its fields.
type ReceiverChannel struct {
	// Enabled reports whether the station receives this channel. All channels
	// disabled is a valid degraded configuration.
	Enabled bool `json:"enabled"`
	// SensitivityDBm is the reference sensitivity, in [-125, -80] dBm.
	SensitivityDBm float64 `json:"sensitivityDbm"`
	// NoisePenaltyDB degrades the sensitivity, in [0, 40] dB.
	NoisePenaltyDB float64 `json:"noisePenaltyDb"`
	// DropProbability is an extra packet-drop probability in [0, 1].
	DropProbability float64 `json:"dropProbability"`
}

// ShadowSector is the simulator wire representation; units and ranges are documented on its fields.
type ShadowSector struct {
	StartDegrees float64 `json:"startDegrees"`
	EndDegrees   float64 `json:"endDegrees"`
	// LossDB is the extra loss inside the sector, in [0, 60] dB.
	LossDB float64 `json:"lossDb"`
}

// ReceptionModel is the simulator wire representation; units and ranges are documented on its fields.
type ReceptionModel struct {
	// SiteLossDB is excess loss in dB on every path.
	SiteLossDB float64 `json:"siteLossDb"`
	// PathExponent adds 10*(PathExponent-2)*log10(km) dB beyond 1 km.
	PathExponent float64 `json:"pathExponent"`
	// EffectiveEarthRadiusFactor scales the earth radius of the radio horizon.
	EffectiveEarthRadiusFactor float64 `json:"effectiveEarthRadiusFactor"`
	ChannelAFrequencyMHz       float64 `json:"channelAFrequencyMhz"`
	ChannelBFrequencyMHz       float64 `json:"channelBFrequencyMhz"`
	// HorizonTaperStart is the fraction of the horizon where probability starts
	// to fall linearly to 0 at the horizon.
	HorizonTaperStart float64 `json:"horizonTaperStart"`
	// Margin probability interpolates linearly through
	// (ZeroProbabilityMarginDB, 0), (0 dB, ReferenceProbability), and
	// (FullProbabilityMarginDB, 1), clamped outside.
	ZeroProbabilityMarginDB float64 `json:"zeroProbabilityMarginDb"`
	ReferenceProbability    float64 `json:"referenceProbability"`
	FullProbabilityMarginDB float64 `json:"fullProbabilityMarginDb"`
	// CoverageThresholds are the probabilities of Station.Coverage contours.
	CoverageThresholds []float64 `json:"coverageThresholds"`
}

// ObservationSettings is the simulator wire representation; units and ranges are documented on its fields.
type ObservationSettings struct {
	// ReceptionHistoryLimit is the number of newest receptions kept per station.
	ReceptionHistoryLimit int `json:"receptionHistoryLimit"`
	// TargetLimit is the number of distinct MMSIs in the observation store.
	TargetLimit int `json:"targetLimit"`
	// RecentReceptionLimit is the size of Observations.RecentReceptions.
	RecentReceptionLimit int `json:"recentReceptionLimit"`
	// FreshAgeMs is the largest fresh age; StaleAgeMs the largest stale age.
	// Older observations are lost until they reach ExpiryAgeMs and are removed.
	FreshAgeMs  int64 `json:"freshAgeMs"`
	StaleAgeMs  int64 `json:"staleAgeMs"`
	ExpiryAgeMs int64 `json:"expiryAgeMs"`
	// RateWindowMs is the longest span of recent station counters.
	RateWindowMs int64 `json:"rateWindowMs"`
}

// ReceiverSnapshot is the simulator wire representation; units and ranges are documented on its fields.
type ReceiverSnapshot struct {
	// Name is the label at reception, not the station's current name.
	Name                string          `json:"name"`
	Latitude            float64         `json:"latitude"`
	Longitude           float64         `json:"longitude"`
	AntennaHeightMeters float64         `json:"antennaHeightMeters"`
	ReceiveGainDBi      float64         `json:"receiveGainDbi"`
	FeederLossDB        float64         `json:"feederLossDb"`
	Channel             ReceiverChannel `json:"channel"`
}

// ReceptionLink is the simulator wire representation; units and ranges are documented on its fields.
type ReceptionLink struct {
	// DistanceMeters is the geodesic distance from station to transmitter.
	DistanceMeters float64 `json:"distanceMeters"`
	// BearingDegrees is the true bearing from station to transmitter in
	// [0, 360), nil at coincident positions.
	BearingDegrees *float64 `json:"bearingDegrees"`
	HorizonMeters  float64  `json:"horizonMeters"`
	// ShadowLossDB is the loss of the shadow sector containing the bearing, or 0.
	ShadowLossDB            float64 `json:"shadowLossDb"`
	ReceivedPowerDBm        float64 `json:"receivedPowerDbm"`
	EffectiveSensitivityDBm float64 `json:"effectiveSensitivityDbm"`
	MarginDB                float64 `json:"marginDb"`
	// Probability is the decode probability the reception draw was compared with.
	Probability float64 `json:"probability"`
}

// ReceptionCounters is the simulator wire representation; units and ranges are documented on its fields.
type ReceptionCounters struct {
	Opportunities uint64 `json:"opportunities,string"`
	Received      uint64 `json:"received,string"`
	// StationDisabled through ProbabilisticLoss are the loss reasons, in
	// precedence order.
	StationDisabled    uint64          `json:"stationDisabled,string"`
	ChannelDisabled    uint64          `json:"channelDisabled,string"`
	OutsideHorizon     uint64          `json:"outsideHorizon,string"`
	InsufficientMargin uint64          `json:"insufficientMargin,string"`
	ProbabilisticLoss  uint64          `json:"probabilisticLoss,string"`
	ChannelA           ChannelCounters `json:"channelA"`
	ChannelB           ChannelCounters `json:"channelB"`
}

// ChannelCounters is the simulator wire representation; units and ranges are documented on its fields.
type ChannelCounters struct {
	Opportunities uint64 `json:"opportunities,string"`
	Received      uint64 `json:"received,string"`
}

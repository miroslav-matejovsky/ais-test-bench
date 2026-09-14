package simulation

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxStations limits the number of configured receiving stations.
	MaxStations = 16
	// MaxStationNameRunes limits a station name. Valid UTF-8 needs at most four
	// bytes per rune, so a name is at most 320 bytes.
	MaxStationNameRunes = 80
	// MaxShadowSectors limits the shadow sectors of one station.
	MaxShadowSectors = 8
)

// firstRevision is the revision of new stations and of the initial station set.
const firstRevision = 1

// interval is an accepted numeric range with its unit, used for validation
// and error messages.
type interval struct {
	low, high         float64
	lowOpen, highOpen bool // Exclude the bound.
	unit              string
}

// Accepted ranges of transmitter and station fields.
var (
	transmitterPowerRange  = interval{low: 0, high: 25, lowOpen: true, unit: "W"}
	transmitterHeightRange = interval{low: 0, high: 100, lowOpen: true, unit: "m"}
	gainRange              = interval{low: -10, high: 20, unit: "dBi"}
	feederLossRange        = interval{low: 0, high: 30, unit: "dB"}
	latitudeRange          = interval{low: -85, high: 85, unit: "degrees"}
	longitudeRange         = interval{low: -180, high: 180, unit: "degrees"}
	stationHeightRange     = interval{low: 0, high: 500, lowOpen: true, unit: "m"}
	sensitivityRange       = interval{low: -125, high: -80, unit: "dBm"}
	noisePenaltyRange      = interval{low: 0, high: 40, unit: "dB"}
	dropProbabilityRange   = interval{low: 0, high: 1}
	bearingRange           = interval{low: 0, high: 360, highOpen: true, unit: "degrees"}
	sectorLossRange        = interval{low: 0, high: 60, unit: "dB"}
)

// check rejects a non-finite value before the range check.
func (r interval) check(field string, v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("%s must be finite: %v", field, v)
	}
	if v < r.low || v > r.high || (r.lowOpen && v == r.low) || (r.highOpen && v == r.high) {
		low, high := "[", "]"
		if r.lowOpen {
			low = "("
		}
		if r.highOpen {
			high = ")"
		}
		return fmt.Errorf("%s must be in %s%v, %v%s %s: %v", field, low, r.low, r.high, high, r.unit, v)
	}
	return nil
}

// stationState is one configured station. definition and coverage are never
// mutated in place; edits replace them, so state copies may share them.
type stationState struct {
	id             string
	definition     StationDefinition
	configRevision uint64
	rfRevision     uint64
	createdAt      time.Time
	rfUpdatedAt    time.Time
	coverage       []Coverage // Computed for the current RF revision.
}

// ValidateStation returns the error AddStation, UpdateStation, and New return
// for definition, or nil when they accept it. Errors wrap ErrInvalid.
func ValidateStation(definition StationDefinition) error {
	_, err := canonicalStation(definition)
	if err != nil {
		return fmt.Errorf("%w: station: %w", ErrInvalid, err)
	}
	return nil
}

// Stations returns a copy of the station configuration.
func (s *Simulator) Stations() StationSet {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stationSet()
}

// StationConfiguration returns station definitions, clock, and settings from
// one committed state. It does not settle time or expose mutable engine storage.
func (s *Simulator) StationConfiguration() StationConfiguration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return StationConfiguration{StationSet: s.stationSet(), Metadata: s.metadata(), StateRevision: s.state.revision}
}

// AddStation adds a station at the current virtual instant and returns its new
// ID with the resulting configuration. The station gets revision 1 for both
// config and RF and zero counters. It receives only later reports, draws no
// vessel randomness, and changes no report.
//
// Errors: an invalid definition wraps ErrInvalid; expectedRevision other than
// the current StationSet.Revision wraps ErrConflict; MaxStations existing
// stations or an exhausted ID, station set revision, or state revision range
// wraps ErrLimit. A failure changes nothing.
func (s *Simulator) AddStation(expectedRevision uint64, definition StationDefinition) (string, StationSet, error) {
	canonical, err := canonicalStation(definition)
	if err != nil {
		return "", StationSet{}, fmt.Errorf("%w: station: %w", ErrInvalid, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkStationRevision(expectedRevision); err != nil {
		return "", StationSet{}, err
	}
	if len(s.state.stations) >= MaxStations {
		return "", StationSet{}, fmt.Errorf("%w: at most %d stations", ErrLimit, MaxStations)
	}
	if s.state.lastStationID == math.MaxUint64 || s.state.stationRevision == math.MaxUint64 || s.state.revision == math.MaxUint64 {
		return "", StationSet{}, fmt.Errorf("%w: station id or revision range exhausted", ErrLimit)
	}
	s.state.lastStationID++
	now := s.start.Add(s.state.elapsed)
	station := stationState{
		id: stationID(s.state.lastStationID), definition: canonical,
		configRevision: firstRevision, rfRevision: firstRevision, createdAt: now, rfUpdatedAt: now,
		coverage: computeCoverage(s.transmitter, canonical),
	}
	s.state.stations = append(s.state.stations, station)
	s.store.stations[station.id] = &stationReceptions{}
	s.state.stationRevision++
	s.state.revision++
	return station.id, s.stationSet(), nil
}

// UpdateStation replaces the definition of station id and returns the resulting
// configuration. An edit equal to the stored canonical definition is a no-op
// that keeps all revisions. Otherwise the config revision and set revision
// increase, and the RF revision increases unless only Name changed. ID and
// creation time never change. Counters, history, and observations are kept;
// later decisions use the new revision.
//
// Errors: an invalid definition wraps ErrInvalid; a stale expectedRevision wraps
// ErrConflict; an unknown id wraps ErrNotFound; an exhausted revision range
// wraps ErrLimit. A failure changes nothing.
func (s *Simulator) UpdateStation(expectedRevision uint64, id string, definition StationDefinition) (StationSet, error) {
	canonical, err := canonicalStation(definition)
	if err != nil {
		return StationSet{}, fmt.Errorf("%w: station: %w", ErrInvalid, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkStationRevision(expectedRevision); err != nil {
		return StationSet{}, err
	}
	i, err := s.findStation(id)
	if err != nil {
		return StationSet{}, err
	}
	station := s.state.stations[i]
	if equalStation(station.definition, canonical) {
		return s.stationSet(), nil
	}
	named := station.definition
	named.Name = canonical.Name
	rfChanged := !equalStation(named, canonical)
	if station.configRevision == math.MaxUint64 || s.state.stationRevision == math.MaxUint64 ||
		s.state.revision == math.MaxUint64 || (rfChanged && station.rfRevision == math.MaxUint64) {
		return StationSet{}, fmt.Errorf("%w: station %s revision range exhausted", ErrLimit, id)
	}
	station.definition = canonical
	station.configRevision++
	if rfChanged {
		station.rfRevision++
		station.rfUpdatedAt = s.start.Add(s.state.elapsed)
		station.coverage = computeCoverage(s.transmitter, canonical)
	}
	s.state.stations[i] = station
	s.state.stationRevision++
	s.state.revision++
	return s.stationSet(), nil
}

// RemoveStation removes station id from the configuration and returns the
// result. Its counters, reception history, and observations are removed; run
// reception totals keep its receptions. Its ID is not reused. Removing the last
// station leaves vessel generation running.
//
// Errors: a stale expectedRevision wraps ErrConflict; an unknown id wraps
// ErrNotFound; an exhausted revision range wraps ErrLimit. A failure changes
// nothing.
func (s *Simulator) RemoveStation(expectedRevision uint64, id string) (StationSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkStationRevision(expectedRevision); err != nil {
		return StationSet{}, err
	}
	i, err := s.findStation(id)
	if err != nil {
		return StationSet{}, err
	}
	if s.state.stationRevision == math.MaxUint64 || s.state.revision == math.MaxUint64 {
		return StationSet{}, fmt.Errorf("%w: station set or state revision range exhausted", ErrLimit)
	}
	s.state.stations = slices.Delete(s.state.stations, i, i+1)
	s.store.removeStation(id)
	s.state.stationRevision++
	s.state.revision++
	return s.stationSet(), nil
}

// newStations validates the initial definitions and assigns IDs in order.
func newStations(definitions []StationDefinition, at time.Time, tx TransmitterProfile) ([]stationState, error) {
	if len(definitions) > MaxStations {
		return nil, fmt.Errorf("%w: station count must be between 0 and %d: %d", ErrInvalid, MaxStations, len(definitions))
	}
	stations := make([]stationState, 0, len(definitions))
	for i, definition := range definitions {
		canonical, err := canonicalStation(definition)
		if err != nil {
			return nil, fmt.Errorf("%w: station %d: %w", ErrInvalid, i, err)
		}
		stations = append(stations, stationState{
			id: stationID(uint64(i + 1)), definition: canonical,
			configRevision: firstRevision, rfRevision: firstRevision, createdAt: at, rfUpdatedAt: at,
			coverage: computeCoverage(tx, canonical),
		})
	}
	return stations, nil
}

// stationID formats allocator number n as an opaque station ID.
func stationID(n uint64) string {
	return "station-" + strconv.FormatUint(n, 10)
}

// stationSet copies the station configuration. Called with the lock held.
func (s *Simulator) stationSet() StationSet {
	stations := make([]Station, 0, len(s.state.stations))
	for _, station := range s.state.stations {
		definition := station.definition
		definition.ShadowSectors = slices.Clone(definition.ShadowSectors)
		coverage := slices.Clone(station.coverage)
		for i := range coverage {
			coverage[i].Ring = slices.Clone(coverage[i].Ring)
		}
		stations = append(stations, Station{
			ID: station.id, Definition: definition,
			ConfigRevision: station.configRevision, RFRevision: station.rfRevision,
			CreatedAt: station.createdAt, RFUpdatedAt: station.rfUpdatedAt, Coverage: coverage,
		})
	}
	return StationSet{SimulationID: s.id, Revision: s.state.stationRevision, Stations: stations}
}

// checkStationRevision rejects a stale expected revision. Called with the lock
// held.
func (s *Simulator) checkStationRevision(expected uint64) error {
	if expected != s.state.stationRevision {
		return fmt.Errorf("%w: station set revision is %d, not %d", ErrConflict, s.state.stationRevision, expected)
	}
	return nil
}

// findStation returns the index of station id. Called with the lock held.
func (s *Simulator) findStation(id string) (int, error) {
	i := slices.IndexFunc(s.state.stations, func(station stationState) bool { return station.id == id })
	if i < 0 {
		return 0, fmt.Errorf("%w: station %q", ErrNotFound, id)
	}
	return i, nil
}

// validateTransmitter checks every transmitter field.
func validateTransmitter(t TransmitterProfile) error {
	err := errors.Join(
		transmitterPowerRange.check("power", t.PowerWatts),
		transmitterHeightRange.check("height", t.HeightMeters),
		gainRange.check("gain", t.GainDBi),
		feederLossRange.check("feeder loss", t.FeederLossDB),
	)
	if err != nil {
		return fmt.Errorf("%w: transmitter: %w", ErrInvalid, err)
	}
	return nil
}

// canonicalStation validates d and returns its canonical form: longitude 180 as
// -180 and a detached copy of the sectors sorted by start. Errors do not wrap
// ErrInvalid; callers add it with their context.
func canonicalStation(d StationDefinition) (StationDefinition, error) {
	errs := []error{
		validateStationName(d.Name),
		latitudeRange.check("latitude", d.Latitude),
		longitudeRange.check("longitude", d.Longitude),
		stationHeightRange.check("antenna height", d.AntennaHeightMeters),
		gainRange.check("receive gain", d.ReceiveGainDBi),
		feederLossRange.check("feeder loss", d.FeederLossDB),
		validateChannel("channel A", d.ChannelA),
		validateChannel("channel B", d.ChannelB),
		validateSectors(d.ShadowSectors),
	}
	if err := errors.Join(errs...); err != nil {
		return StationDefinition{}, err
	}
	if d.Longitude == 180 {
		d.Longitude = -180
	}
	d.ShadowSectors = slices.Clone(d.ShadowSectors)
	slices.SortFunc(d.ShadowSectors, func(a, b ShadowSector) int { return cmp.Compare(a.StartDegrees, b.StartDegrees) })
	return d, nil
}

func validateStationName(name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errors.New("name is required")
	case !utf8.ValidString(name):
		return errors.New("name must be valid UTF-8")
	case utf8.RuneCountInString(name) > MaxStationNameRunes:
		return fmt.Errorf("name must have at most %d characters: %d", MaxStationNameRunes, utf8.RuneCountInString(name))
	}
	return nil
}

func validateChannel(name string, c ReceiverChannel) error {
	err := errors.Join(
		sensitivityRange.check("sensitivity", c.SensitivityDBm),
		noisePenaltyRange.check("noise penalty", c.NoisePenaltyDB),
		dropProbabilityRange.check("drop probability", c.DropProbability),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// validateSectors checks sector count, fields, and overlap. Adjacent sectors
// share only an exclusive end and do not overlap.
func validateSectors(sectors []ShadowSector) error {
	if len(sectors) > MaxShadowSectors {
		return fmt.Errorf("at most %d shadow sectors: %d", MaxShadowSectors, len(sectors))
	}
	// span is one half-open bearing interval; a sector wrapping north has two.
	type span struct {
		sector     int
		start, end float64
	}
	spans := make([]span, 0, 2*len(sectors))
	for i, sector := range sectors {
		field := fmt.Sprintf("shadow sector %d", i)
		err := errors.Join(
			bearingRange.check(field+" start", sector.StartDegrees),
			bearingRange.check(field+" end", sector.EndDegrees),
			sectorLossRange.check(field+" loss", sector.LossDB),
		)
		if err != nil {
			return err
		}
		switch {
		case sector.StartDegrees == sector.EndDegrees:
			return fmt.Errorf("%s start and end must differ: %v", field, sector.StartDegrees)
		case sector.StartDegrees < sector.EndDegrees:
			spans = append(spans, span{i, sector.StartDegrees, sector.EndDegrees})
		default:
			spans = append(spans, span{i, sector.StartDegrees, 360}, span{i, 0, sector.EndDegrees})
		}
	}
	for i, a := range spans {
		for _, b := range spans[i+1:] {
			if a.sector != b.sector && a.start < b.end && b.start < a.end {
				return fmt.Errorf("shadow sectors %d and %d overlap", a.sector, b.sector)
			}
		}
	}
	return nil
}

// equalStation compares canonical definitions field by field.
func equalStation(a, b StationDefinition) bool {
	return a.Name == b.Name && a.Latitude == b.Latitude && a.Longitude == b.Longitude &&
		a.Enabled == b.Enabled && a.AntennaHeightMeters == b.AntennaHeightMeters &&
		a.ReceiveGainDBi == b.ReceiveGainDBi && a.FeederLossDB == b.FeederLossDB &&
		a.ChannelA == b.ChannelA && a.ChannelB == b.ChannelB &&
		slices.Equal(a.ShadowSectors, b.ShadowSectors)
}

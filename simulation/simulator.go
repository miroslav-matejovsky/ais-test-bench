package simulation

import (
	"context"
	"errors"
	"fmt"
	"math"
	mathrand "math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/ais"
)

const (
	// MaxVessels limits the fleet size accepted by New and SetCount.
	MaxVessels = 100
	// MessageLimit is the number of latest AIS reports retained in History.
	MessageLimit = 1000
	// TickInterval is the virtual report cadence. Ticks fall on the start instant
	// plus every positive multiple of TickInterval.
	TickInterval = time.Second
	// MaxAdvance is the largest virtual duration one Advance or Elapse call
	// covers. At MaxVessels one call returns at most 6,000 tick reports.
	MaxAdvance = 60 * time.Second
	// MinSpeed is the slowest running speed; 0 pauses Elapse.
	MinSpeed = 0.01
	// MaxSpeed is the fastest speed.
	MaxSpeed = 100.0
	// SpeedStep is the speed precision.
	SpeedStep = 0.01
)

var (
	// ErrInvalid marks rejected input: configuration, count, speed, station
	// definition, or a negative duration.
	ErrInvalid = errors.New("invalid simulation input")
	// ErrLimit marks a request beyond engine limits: an advance over MaxAdvance,
	// a virtual time after year 9999, more than MaxStations, or an exhausted
	// duration, sequence, station ID, or revision range.
	ErrLimit = errors.New("simulation limit exceeded")
	// ErrNotFound marks a station ID that is not configured.
	ErrNotFound = errors.New("simulation station not found")
	// ErrConflict marks a station edit based on a stale station set revision.
	ErrConflict = errors.New("simulation station configuration conflict")
)

// Generation settings. Metadata derives its values from these constants.
const (
	firstMMSI      = 200000000
	cargoTypeID    = "cargo"
	spawnSouth     = 52.0
	spawnLatSpan   = 0.04
	spawnWest      = 3.94
	spawnLonSpan   = 0.06
	minSpeedKnots  = 6.0
	speedSteps     = 100 // Speeds are minSpeedKnots plus 0-99 tenths of a knot.
	positionReport = 1   // AIS message type emitted for every report.
)

// Clock settings.
const (
	speedScale     = 100  // Speeds are stored as integer hundredths.
	speedTolerance = 1e-6 // Accepted float error in hundredths, e.g. for 0.1+0.2.
	minYear        = 1    // Virtual dates stay within the JSON-compatible range.
	maxYear        = 9999
)

// vesselTypes is the application vessel category catalog.
var vesselTypes = []VesselType{{ID: cargoTypeID, Name: "Cargo vessel"}}

// vesselState is one active vessel: navigation state, stable synthetic identity,
// and latest report. Invariant: Position.UpdatedAt is the latest tick or birth
// time, report is the encoding of the current Position, and the same message is
// in history until evicted.
type vesselState struct {
	ais.Position
	name    string
	typeID  string
	report  Message
	reports uint64 // Reports emitted so far; selects the next channel.
}

// channel returns the channel of the vessel's next report. Channels alternate
// per vessel, starting on A for an even MMSI and on B for an odd one, so every
// vessel uses both channels whatever the fleet size.
func (v *vesselState) channel() Channel {
	if (uint64(v.MMSI)+v.reports)%2 == 0 {
		return ChannelA
	}
	return ChannelB
}

// state is all mutable engine state except history and reception state. A
// mutation copies it, changes only the copy, stages its receptions, and commits
// the copy with its reports and receptions after every step succeeded, so a
// failure leaves no trace, including in future random draws.
type state struct {
	revision  uint64        // State revision, 1 after New.
	source    mathrand.PCG  // Held by value so a staged copy draws independently.
	vessels   []vesselState // Creation order.
	nextMMSI  uint32
	sequence  uint64        // Last emitted report sequence; 0 before the first report.
	elapsed   time.Duration // Committed virtual time since the start instant.
	remainder int64         // Scaled real time below 1ns, in hundredths of a nanosecond (0-99).
	speed     int64         // Elapse multiplier in hundredths; 0 pauses.
	updatedAt time.Time     // Latest report tick or effective count change.

	stations        []stationState // Creation order.
	lastStationID   uint64         // Last allocated station number; IDs are never reused.
	stationRevision uint64         // Station set revision, starting at 1.
}

// Simulator owns vessels, its random source, its virtual clock, and recent
// messages under one lock. Construct it with New. It starts no goroutine and
// reads no clock.
type Simulator struct {
	// id, start, seed, initialCount, and transmitter are immutable after New
	// and read without the lock.
	id           string
	start        time.Time
	seed         uint64
	initialCount int
	transmitter  TransmitterProfile

	mu       sync.Mutex
	state    state
	messages []Message // Oldest first, at most MessageLimit.
	store    receptionStore
}

// New validates config and starts a run with InitialVesselCount randomly
// positioned vessels, reported at StartTime and available through Fleet and
// History. Validation happens before any random draw; errors wrap ErrInvalid.
func New(config Config) (*Simulator, error) {
	if config.ID == "" {
		return nil, fmt.Errorf("%w: simulation id is required", ErrInvalid)
	}
	if config.StartTime.IsZero() {
		return nil, fmt.Errorf("%w: start time is required", ErrInvalid)
	}
	start := config.StartTime.UTC()
	if start.Year() < minYear || start.Year() > maxYear {
		return nil, fmt.Errorf("%w: start time %s is outside years %d-%d", ErrInvalid, start.Format(time.RFC3339Nano), minYear, maxYear)
	}
	if err := checkCount(config.InitialVesselCount); err != nil {
		return nil, err
	}
	speed, err := normalizeSpeed(config.Speed)
	if err != nil {
		return nil, err
	}
	if err := validateTransmitter(config.Transmitter); err != nil {
		return nil, err
	}
	stations, err := newStations(config.Stations, start, config.Transmitter)
	if err != nil {
		return nil, err
	}
	s := &Simulator{
		id: config.ID, start: start, seed: config.Seed, initialCount: config.InitialVesselCount,
		transmitter: config.Transmitter,
		state: state{
			source:  *mathrand.NewPCG(config.Seed, config.Seed^0xa15),
			vessels: make([]vesselState, 0), nextMMSI: firstMMSI,
			speed: speed, updatedAt: start,
			stations: stations, lastStationID: uint64(len(stations)), stationRevision: firstRevision,
		},
		messages: make([]Message, 0),
		store:    newReceptionStore(stations),
	}
	if _, err := s.SetCount(config.InitialVesselCount); err != nil {
		return nil, err
	}
	s.state.revision = firstRevision
	return s, nil
}

// SetCount adjusts the active fleet at the current virtual time and returns the
// creation reports of added vessels in creation order. Every station evaluates
// the creation reports. Existing vessels keep their identity, navigation, and
// report; new vessels move only from their birth time. Removed vessels' history
// and observations remain until evicted or expired. An effective change sets
// Fleet.UpdatedAt; an unchanged count changes nothing. Tick scheduling is not
// affected. Count must be 0 through MaxVessels (ErrInvalid); an exhausted
// sequence or revision range wraps ErrLimit. A failure returns no reports and
// changes nothing.
func (s *Simulator) SetCount(count int) ([]Message, error) {
	if err := checkCount(count); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.stage()
	now := s.start.Add(next.elapsed)
	added := count - len(next.vessels)
	reports := make([]Message, 0, max(added, 0))
	if added == 0 {
		return reports, nil
	}
	if err := next.revise(); err != nil {
		return nil, err
	}
	batch := s.stageReceptions(next.stations)
	if added < 0 {
		next.vessels = next.vessels[:count]
		next.updatedAt = now
	}
	if added > 0 {
		if err := next.reserve(uint64(added)); err != nil {
			return nil, err
		}
		random := mathrand.New(&next.source)
		for range added {
			mmsi := next.nextMMSI
			course := float64(random.IntN(360))
			vessel := vesselState{Position: ais.Position{
				MMSI:      mmsi,
				Latitude:  spawnSouth + random.Float64()*spawnLatSpan,
				Longitude: spawnWest + random.Float64()*spawnLonSpan,
				Speed:     minSpeedKnots + float64(random.IntN(speedSteps))/10,
				Course:    course, Heading: int(course), UpdatedAt: now,
			}, name: fmt.Sprintf("Vessel %d", mmsi-firstMMSI+1), typeID: cargoTypeID}
			t, err := next.emit(&vessel)
			if err != nil {
				return nil, err
			}
			if err := s.receive(&batch, next.stations, t); err != nil {
				return nil, err
			}
			next.vessels = append(next.vessels, vessel)
			next.nextMMSI++
			reports = append(reports, t.Message)
		}
		next.updatedAt = now
	}
	s.commit(next, reports, batch)
	return reports, nil
}

// Advance moves virtual time forward by exactly virtualDelta, independent of
// speed, and returns every report it emits in sequence order, including reports
// already evicted from History. Every tick boundary after the current instant
// and up to and including the target moves each vessel from its previous tick or
// birth time to the boundary along a great circle and reports it, in creation
// order. Every station evaluates every report. A target between boundaries
// advances the clock without a partial tick. The clock, tick schedule, and
// observation ages also advance with an empty fleet. Advance works while paused
// and keeps the Elapse scaling remainder.
//
// Zero is a no-op returning an empty slice. A negative duration wraps
// ErrInvalid. More than MaxAdvance, a target after year 9999, or exhausted
// duration, sequence, reception sequence, or revision range wraps ErrLimit. ctx
// is checked before the first tick, between ticks, and before commit;
// cancellation returns its error. Every failure returns no reports and leaves
// the engine unchanged, including reception state.
func (s *Simulator) Advance(ctx context.Context, virtualDelta time.Duration) ([]Message, error) {
	if virtualDelta < 0 {
		return nil, fmt.Errorf("%w: virtual duration must not be negative: %v", ErrInvalid, virtualDelta)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.advance(ctx, s.stage(), virtualDelta)
}

// Elapse converts realDelta to virtual time at the current speed and advances
// like Advance, returning every emitted report. Scaling is exact integer
// arithmetic in hundredths of a nanosecond; the part below one nanosecond carries
// to later Elapse calls, so split calls equal one combined call. At speed 0 the
// real time is discarded: no reports, no clock change, and no catch-up after
// resuming. The scaled duration must not exceed MaxAdvance; callers split longer
// periods. Errors and atomicity are those of Advance.
func (s *Simulator) Elapse(ctx context.Context, realDelta time.Duration) ([]Message, error) {
	if realDelta < 0 {
		return nil, fmt.Errorf("%w: elapsed duration must not be negative: %v", ErrInvalid, realDelta)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.stage()
	if next.speed > 0 && int64(realDelta) > (math.MaxInt64-next.remainder)/next.speed {
		return nil, fmt.Errorf("%w: elapsed duration %v overflows at speed %v", ErrLimit, realDelta, float64(next.speed)/speedScale)
	}
	scaled := int64(realDelta)*next.speed + next.remainder
	next.remainder = scaled % speedScale
	return s.advance(ctx, next, time.Duration(scaled/speedScale))
}

// SetSpeed sets the Elapse multiplier: 0 pauses, otherwise MinSpeed through
// MaxSpeed in SpeedStep increments. Float values within binary rounding of a
// step are normalized to it. Only future Elapse scaling changes; the clock, tick
// schedule, reports, and carried remainder are kept, and no report is emitted.
// Invalid values wrap ErrInvalid and an exhausted revision range wraps ErrLimit;
// both change nothing.
func (s *Simulator) SetSpeed(speed float64) error {
	hundredths, err := normalizeSpeed(speed)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if hundredths == s.state.speed {
		return nil
	}
	if err := s.state.revise(); err != nil {
		return err
	}
	s.state.speed = hundredths
	return nil
}

// ValidateSpeed returns the error SetSpeed and New return for speed, or nil when
// they accept it. Errors wrap ErrInvalid.
func ValidateSpeed(speed float64) error {
	_, err := normalizeSpeed(speed)
	return err
}

// advance moves the staged clock by delta, emits every due tick, and commits.
// Called with the lock held.
func (s *Simulator) advance(ctx context.Context, next state, delta time.Duration) ([]Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("advance simulation: %w", err)
	}
	if delta > MaxAdvance {
		return nil, fmt.Errorf("%w: virtual advance %v exceeds %v", ErrLimit, delta, MaxAdvance)
	}
	if next.elapsed > math.MaxInt64-delta {
		return nil, fmt.Errorf("%w: virtual elapsed time overflows", ErrLimit)
	}
	target := next.elapsed + delta
	if at := s.start.Add(target); at.Year() > maxYear {
		return nil, fmt.Errorf("%w: virtual time %s is after year %d", ErrLimit, at.Format(time.RFC3339Nano), maxYear)
	}
	// Tick n is at start + n*TickInterval. Ticks up to next.elapsed are done.
	first, last := next.elapsed/TickInterval+1, target/TickInterval
	ticks := max(last-first+1, 0)
	if err := next.reserve(uint64(ticks) * uint64(len(next.vessels))); err != nil {
		return nil, err
	}
	if delta > 0 {
		if err := next.revise(); err != nil {
			return nil, err
		}
	}
	batch := s.stageReceptions(next.stations)
	reports := make([]Message, 0, int(ticks)*len(next.vessels))
	// One tick is a bounded chunk: at most MaxVessels reports, each evaluated at
	// most MaxStations times.
	for tick := first; tick <= last; tick++ {
		if tick > first {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("advance simulation: %w", err)
			}
		}
		at := s.start.Add(tick * TickInterval)
		for i := range next.vessels {
			vessel := &next.vessels[i]
			move(&vessel.Position, at)
			t, err := next.emit(vessel)
			if err != nil {
				return nil, err
			}
			if err := s.receive(&batch, next.stations, t); err != nil {
				return nil, err
			}
			reports = append(reports, t.Message)
		}
		next.updatedAt = at
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("advance simulation: %w", err)
	}
	next.elapsed = target
	s.commit(next, reports, batch)
	return reports, nil
}

// move advances p along a great circle at its fixed speed and course to at.
func move(p *ais.Position, at time.Time) {
	latitude, longitude := destination(p.Latitude, p.Longitude, p.Course, p.Speed*1852*at.Sub(p.UpdatedAt).Hours())
	p.Latitude = latitude
	p.Longitude = math.Mod(longitude+540, 360) - 180
	p.UpdatedAt = at
}

// stage returns a copy of the state that a mutation can change freely.
func (s *Simulator) stage() state {
	next := s.state
	next.vessels = slices.Clone(s.state.vessels)
	next.stations = slices.Clone(s.state.stations)
	return next
}

// commit publishes a successful mutation: it replaces the state, appends
// reports to history, evicting the oldest beyond MessageLimit, and applies the
// staged receptions at the committed instant. It cannot fail.
func (s *Simulator) commit(next state, reports []Message, batch receptionBatch) {
	s.state = next
	s.messages = append(s.messages, reports...)
	if excess := len(s.messages) - MessageLimit; excess > 0 {
		s.messages = append(s.messages[:0], s.messages[excess:]...)
	}
	s.applyReceptions(next.stations, batch, s.start.Add(next.elapsed))
}

// reserve fails when n more reports would exhaust the sequence range.
func (st *state) reserve(n uint64) error {
	if n > math.MaxUint64-st.sequence {
		return fmt.Errorf("%w: %d more reports exceed the sequence range", ErrLimit, n)
	}
	return nil
}

// revise increments the state revision, failing when its range is exhausted.
func (st *state) revise() error {
	if st.revision == math.MaxUint64 {
		return fmt.Errorf("%w: state revision range exhausted", ErrLimit)
	}
	st.revision++
	return nil
}

// emit encodes the current position of v on its next channel with the next
// staged sequence, makes it the vessel's latest report, and returns it with
// the data reception needs.
func (st *state) emit(v *vesselState) (transmission, error) {
	channel := v.channel()
	sentence, err := ais.EncodePosition(v.Position, ais.Channel(channel))
	if err != nil {
		return transmission{}, fmt.Errorf("encode vessel %d: %w", v.MMSI, err)
	}
	st.sequence++
	v.reports++
	v.report = Message{Sequence: st.sequence, MMSI: v.MMSI, Timestamp: v.UpdatedAt, Sentence: sentence}
	return transmission{
		Message: v.report, channel: channel, latitude: v.Latitude, longitude: v.Longitude,
		name: v.name, typeID: v.typeID,
	}, nil
}

func checkCount(count int) error {
	if count < 0 || count > MaxVessels {
		return fmt.Errorf("%w: vessel count must be between 0 and %d: %d", ErrInvalid, MaxVessels, count)
	}
	return nil
}

// normalizeSpeed validates speed and returns it in integer hundredths.
func normalizeSpeed(speed float64) (int64, error) {
	if math.IsNaN(speed) || speed < 0 || speed > MaxSpeed {
		return 0, fmt.Errorf("%w: speed must be 0 or between %v and %v: %v", ErrInvalid, MinSpeed, MaxSpeed, speed)
	}
	scaled := speed * speedScale
	hundredths := math.Round(scaled)
	if math.Abs(scaled-hundredths) > speedTolerance || (hundredths == 0 && speed != 0) {
		return 0, fmt.Errorf("%w: speed must be a multiple of %v: %v", ErrInvalid, SpeedStep, speed)
	}
	return int64(hundredths), nil
}

// Fleet returns a copy of the active fleet with each vessel's latest report.
func (s *Simulator) Fleet() Fleet {
	s.mu.Lock()
	defer s.mu.Unlock()
	vessels := make([]Vessel, 0, len(s.state.vessels))
	for _, vessel := range s.state.vessels {
		vessels = append(vessels, Vessel{
			MMSI: vessel.MMSI, Name: vessel.name, TypeID: vessel.typeID,
			Report: Report{Sequence: vessel.report.Sequence, Timestamp: vessel.report.Timestamp, Sentence: vessel.report.Sentence},
		})
	}
	return Fleet{
		SimulationID: s.id, UpdatedAt: s.state.updatedAt,
		MessageCount: len(s.messages), MessageLimit: MessageLimit, Vessels: vessels,
	}
}

// History returns a copy of the retained reports, ordered oldest first.
func (s *Simulator) History() History {
	s.mu.Lock()
	defer s.mu.Unlock()
	history := History{
		SimulationID: s.id, MessageLimit: MessageLimit,
		Messages: append([]Message{}, s.messages...),
	}
	if len(s.messages) > 0 {
		oldest, latest := s.messages[0].Sequence, s.messages[len(s.messages)-1].Sequence
		history.OldestSequence, history.LatestSequence = &oldest, &latest
	}
	return history
}

// Metadata returns the run identity, committed clock, vessel type catalog, and
// the settings the engine actually uses.
func (s *Simulator) Metadata() Metadata {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metadata()
}

// metadata builds Metadata. Called with the lock held.
func (s *Simulator) metadata() Metadata {
	elapsed, speed := s.state.elapsed, s.state.speed
	return Metadata{
		SimulationID: s.id,
		StartedAt:    s.start,
		Time: TimeState{
			Now: s.start.Add(elapsed), Elapsed: elapsed,
			Speed: float64(speed) / speedScale, Paused: speed == 0,
		},
		VesselTypes:           slices.Clone(vesselTypes),
		SupportedMessageTypes: []int{positionReport},
		Settings: Settings{
			InitialVesselCount:  s.initialCount,
			MaxVessels:          MaxVessels,
			TickIntervalMs:      TickInterval.Milliseconds(),
			MessageIntervalMs:   TickInterval.Milliseconds(),
			MessageHistoryLimit: MessageLimit,
			SpeedKnots:          SpeedRange{Min: minSpeedKnots, Max: minSpeedKnots + (speedSteps-1)/10.0},
			Speed:               SpeedLimits{Min: MinSpeed, Max: MaxSpeed, Step: SpeedStep},
			SpawnBounds: SpawnBounds{
				South: spawnSouth, North: spawnSouth + spawnLatSpan,
				West: spawnWest, East: spawnWest + spawnLonSpan,
			},
			MaxStations: MaxStations,
			Transmitter: s.transmitter,
			Reception: ReceptionModel{
				SiteLossDB: siteLossDB, PathExponent: pathExponent,
				EffectiveEarthRadiusFactor: effectiveEarthRadiusFactor,
				ChannelAFrequencyMHz:       channelAFrequencyMHz, ChannelBFrequencyMHz: channelBFrequencyMHz,
				HorizonTaperStart:       horizonTaperStart,
				ZeroProbabilityMarginDB: zeroProbabilityMarginDB, ReferenceProbability: referenceProbability,
				FullProbabilityMarginDB: fullProbabilityMarginDB,
				CoverageThresholds:      slices.Clone(coverageThresholds),
			},
			Observation: ObservationSettings{
				ReceptionHistoryLimit: ReceptionHistoryLimit, TargetLimit: TargetLimit,
				RecentReceptionLimit: RecentReceptionLimit,
				FreshAgeMs:           FreshAge.Milliseconds(), StaleAgeMs: StaleAge.Milliseconds(),
				ExpiryAgeMs: ExpiryAge.Milliseconds(), RateWindowMs: RateWindow.Milliseconds(),
			},
		},
	}
}

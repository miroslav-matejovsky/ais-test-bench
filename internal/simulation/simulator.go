package simulation

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	mathrand "math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/ais"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
)

const (
	// MaxVessels limits the workload of this local simulator.
	MaxVessels = 100
	// MessageLimit is the number of latest AIS reports retained in memory.
	MessageLimit = 1000
	// InitialVesselCount is the fleet size created by New.
	InitialVesselCount = 1
	// TickInterval is the movement and report cadence used by Run.
	TickInterval = time.Second
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

// vesselTypes is the application vessel category catalog.
var vesselTypes = []simulatorapi.VesselType{{ID: cargoTypeID, Name: "Cargo vessel"}}

// vesselState is one active vessel: navigation state, stable synthetic identity,
// and latest report. Invariant: report is the encoding of the current Position,
// and the same message is in history until evicted.
type vesselState struct {
	ais.Position
	name   string
	typeID string
	report simulatorapi.Message
}

// Simulator owns vessels, its random source, and recent messages under one lock.
// Every mutation prepares and encodes all reports before publishing any state,
// so a failed update leaves vessels, reports, history, and timestamps unchanged.
// Construct it with New, then call Run once for its lifetime.
type Simulator struct {
	// id and startedAt are immutable after New and read without the lock.
	id        string
	startedAt time.Time

	mu        sync.Mutex
	random    *mathrand.Rand
	vessels   []vesselState
	messages  []simulatorapi.Message // Oldest first, at most MessageLimit.
	nextMMSI  uint32
	sequence  uint64 // Last emitted report sequence; 0 before the first report.
	updatedAt time.Time
}

// NewID returns a random opaque simulation identity. Call it once per engine
// start so runs using the same seed remain distinguishable.
func NewID() string {
	return rand.Text()
}

// New starts simulation run id with InitialVesselCount randomly positioned
// vessels and their first AIS reports. The identity, seed, and time are
// explicit so tests can reproduce a run.
func New(id string, now time.Time, seed uint64) (*Simulator, error) {
	if id == "" {
		return nil, fmt.Errorf("simulation id is required")
	}
	s := &Simulator{
		id: id, startedAt: now.UTC(),
		random:  mathrand.New(mathrand.NewPCG(seed, seed^0xa15)),
		vessels: make([]vesselState, 0), messages: make([]simulatorapi.Message, 0),
		nextMMSI: firstMMSI, updatedAt: now.UTC(),
	}
	if err := s.SetCount(InitialVesselCount, now); err != nil {
		return nil, err
	}
	return s, nil
}

// SetCount adjusts the active fleet, retaining existing vessels where possible.
// Newly added vessels emit an immediate report. Removed vessels' history remains
// until it ages out. Zero pauses generation. An effective change advances the
// update time; an unchanged count does not. Count must be 0 through MaxVessels.
func (s *Simulator) SetCount(count int, now time.Time) error {
	if count < 0 || count > MaxVessels {
		return fmt.Errorf("vessel count must be between 0 and %d", MaxVessels)
	}
	if now.IsZero() {
		return fmt.Errorf("simulation time is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if count <= len(s.vessels) {
		if count < len(s.vessels) {
			s.vessels = s.vessels[:count]
			s.publish(nil, now)
		}
		return nil
	}
	added := make([]vesselState, 0, count-len(s.vessels))
	reports := make([]simulatorapi.Message, 0, cap(added))
	mmsi := s.nextMMSI
	for range cap(added) {
		course := float64(s.random.IntN(360))
		vessel := vesselState{Position: ais.Position{
			MMSI:      mmsi,
			Latitude:  spawnSouth + s.random.Float64()*spawnLatSpan,
			Longitude: spawnWest + s.random.Float64()*spawnLonSpan,
			Speed:     minSpeedKnots + float64(s.random.IntN(speedSteps))/10,
			Course:    course, Heading: int(course), UpdatedAt: now.UTC(),
		}, name: fmt.Sprintf("Vessel %d", mmsi-firstMMSI+1), typeID: cargoTypeID}
		report, err := encode(vessel.Position, s.sequence+uint64(len(reports))+1)
		if err != nil {
			return err
		}
		vessel.report = report
		added = append(added, vessel)
		reports = append(reports, report)
		mmsi++
	}
	s.vessels = append(s.vessels, added...)
	s.nextMMSI = mmsi
	s.publish(reports, now)
	return nil
}

// Run advances the simulation every TickInterval until cancellation or an
// encoding error. The simplified reporting cadence is intended for live UI
// development.
func (s *Simulator) Run(ctx context.Context) error {
	ticker := time.NewTicker(TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if err := s.Advance(now); err != nil {
				return err
			}
		}
	}
}

// Advance moves vessels along a great circle at their fixed random speed and
// course, then records a report for each. Times not after the latest update are
// ignored. The update time advances even when the fleet is empty.
func (s *Simulator) Advance(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !now.After(s.updatedAt) {
		return nil
	}
	next := slices.Clone(s.vessels)
	reports := make([]simulatorapi.Message, 0, len(next))
	for i := range next {
		vessel := &next[i]
		if !now.After(vessel.UpdatedAt) {
			continue
		}
		lat := vessel.Latitude * math.Pi / 180
		lon := vessel.Longitude * math.Pi / 180
		bearing := vessel.Course * math.Pi / 180
		distance := vessel.Speed * 1852 * now.Sub(vessel.UpdatedAt).Hours() / 6371000
		nextLat := math.Asin(math.Sin(lat)*math.Cos(distance) + math.Cos(lat)*math.Sin(distance)*math.Cos(bearing))
		nextLon := lon + math.Atan2(math.Sin(bearing)*math.Sin(distance)*math.Cos(lat), math.Cos(distance)-math.Sin(lat)*math.Sin(nextLat))
		vessel.Latitude = nextLat * 180 / math.Pi
		vessel.Longitude = math.Mod(nextLon*180/math.Pi+540, 360) - 180
		vessel.UpdatedAt = now.UTC()
		report, err := encode(vessel.Position, s.sequence+uint64(len(reports))+1)
		if err != nil {
			return err
		}
		vessel.report = report
		reports = append(reports, report)
	}
	s.vessels = next
	s.publish(reports, now)
	return nil
}

// encode prepares the report for p with the given sequence without publishing it.
func encode(p ais.Position, sequence uint64) (simulatorapi.Message, error) {
	sentence, err := ais.EncodePosition(p)
	if err != nil {
		return simulatorapi.Message{}, fmt.Errorf("encode vessel %d: %w", p.MMSI, err)
	}
	return simulatorapi.Message{Sequence: sequence, MMSI: p.MMSI, Timestamp: p.UpdatedAt, Sentence: sentence}, nil
}

// publish appends prepared reports to history, evicting the oldest beyond
// MessageLimit, and moves the update time forward. Called with the lock held
// after every report of the mutation was encoded.
func (s *Simulator) publish(reports []simulatorapi.Message, now time.Time) {
	if len(reports) > 0 {
		s.sequence = reports[len(reports)-1].Sequence
	}
	s.messages = append(s.messages, reports...)
	if excess := len(s.messages) - MessageLimit; excess > 0 {
		s.messages = append(s.messages[:0], s.messages[excess:]...)
	}
	if now.After(s.updatedAt) {
		s.updatedAt = now.UTC()
	}
}

// Fleet returns a copy of the active fleet with each vessel's latest report.
func (s *Simulator) Fleet() simulatorapi.Fleet {
	s.mu.Lock()
	defer s.mu.Unlock()
	vessels := make([]simulatorapi.Vessel, 0, len(s.vessels))
	for _, vessel := range s.vessels {
		vessels = append(vessels, simulatorapi.Vessel{
			MMSI: vessel.MMSI, Name: vessel.name, TypeID: vessel.typeID,
			Report: simulatorapi.Report{Sequence: vessel.report.Sequence, Timestamp: vessel.report.Timestamp, Sentence: vessel.report.Sentence},
		})
	}
	return simulatorapi.Fleet{
		SimulationID: s.id, UpdatedAt: s.updatedAt,
		MessageCount: len(s.messages), MessageLimit: MessageLimit, Vessels: vessels,
	}
}

// History returns a copy of the retained reports, ordered oldest first.
func (s *Simulator) History() simulatorapi.History {
	s.mu.Lock()
	defer s.mu.Unlock()
	history := simulatorapi.History{
		SimulationID: s.id, MessageLimit: MessageLimit,
		Messages: append([]simulatorapi.Message{}, s.messages...),
	}
	if len(s.messages) > 0 {
		oldest, latest := s.messages[0].Sequence, s.messages[len(s.messages)-1].Sequence
		history.OldestSequence, history.LatestSequence = &oldest, &latest
	}
	return history
}

// Metadata returns the run identity, vessel type catalog, and the settings the
// engine actually uses.
func (s *Simulator) Metadata() simulatorapi.Metadata {
	return simulatorapi.Metadata{
		SimulationID:          s.id,
		StartedAt:             s.startedAt,
		VesselTypes:           slices.Clone(vesselTypes),
		SupportedMessageTypes: []int{positionReport},
		Settings: simulatorapi.Settings{
			InitialVesselCount:  InitialVesselCount,
			MaxVessels:          MaxVessels,
			TickIntervalMs:      TickInterval.Milliseconds(),
			MessageIntervalMs:   TickInterval.Milliseconds(),
			MessageHistoryLimit: MessageLimit,
			SpeedKnots:          simulatorapi.SpeedRange{Min: minSpeedKnots, Max: minSpeedKnots + (speedSteps-1)/10.0},
			SpawnBounds: simulatorapi.SpawnBounds{
				South: spawnSouth, North: spawnSouth + spawnLatSpan,
				West: spawnWest, East: spawnWest + spawnLonSpan,
			},
		},
	}
}

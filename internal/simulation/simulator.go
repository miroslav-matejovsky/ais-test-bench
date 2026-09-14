package simulation

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/ais"
)

const (
	// MaxVessels limits the workload of this local simulator.
	MaxVessels = 100
	// MessageLimit is the number of latest AIS reports retained in memory.
	MessageLimit = 1000
)

// Vessel combines a stable synthetic identity with its latest navigation data.
type Vessel struct {
	ais.Position
	Name string `json:"name"`
}

// Message is a generated NMEA sentence, including CRLF, and its source metadata.
type Message struct {
	MMSI      uint32    `json:"mmsi"`
	Timestamp time.Time `json:"timestamp"`
	Sentence  string    `json:"sentence"`
}

// Snapshot is a consistent view of active vessels and retained message count.
type Snapshot struct {
	Vessels      []Vessel  `json:"vessels"`
	MessageCount int       `json:"messageCount"`
	MessageLimit int       `json:"messageLimit"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Simulator owns vessels, its random source, and recent messages under one lock.
// Construct it with New, then call Run once for its lifetime.
type Simulator struct {
	mu        sync.Mutex
	random    *rand.Rand
	vessels   []Vessel
	messages  []Message
	nextMMSI  uint32
	updatedAt time.Time
}

// New starts with one randomly positioned vessel and its first AIS report.
// The seed and time are explicit so simulation tests can reproduce a run.
func New(now time.Time, seed uint64) (*Simulator, error) {
	s := &Simulator{
		random:  rand.New(rand.NewPCG(seed, seed^0xa15)),
		vessels: make([]Vessel, 0), messages: make([]Message, 0),
		nextMMSI: 200000000, updatedAt: now.UTC(),
	}
	if err := s.SetCount(1, now); err != nil {
		return nil, err
	}
	return s, nil
}

// SetCount adjusts the active fleet, retaining existing vessels where possible.
// Newly added vessels emit an immediate report. Removed vessels' history remains
// until it ages out. Zero pauses generation. Count must be 0 through MaxVessels.
func (s *Simulator) SetCount(count int, now time.Time) error {
	if count < 0 || count > MaxVessels {
		return fmt.Errorf("vessel count must be between 0 and %d", MaxVessels)
	}
	if now.IsZero() {
		return fmt.Errorf("simulation time is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.vessels) < count {
		course := float64(s.random.IntN(360))
		vessel := Vessel{Position: ais.Position{
			MMSI:      s.nextMMSI,
			Latitude:  52 + s.random.Float64()*0.04,
			Longitude: 3.94 + s.random.Float64()*0.06,
			Speed:     6 + float64(s.random.IntN(100))/10,
			Course:    course, Heading: int(course), UpdatedAt: now.UTC(),
		}, Name: fmt.Sprintf("Vessel %d", s.nextMMSI-199999999)}
		if err := s.record(vessel); err != nil {
			return err
		}
		s.vessels = append(s.vessels, vessel)
		s.nextMMSI++
	}
	s.vessels = s.vessels[:count]
	return nil
}

// Run advances the simulation every second until cancellation or an encoding
// error. The simplified reporting cadence is intended for live UI development.
func (s *Simulator) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
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
// course, then records a report. Repeated or older tick times are ignored.
func (s *Simulator) Advance(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !now.After(s.updatedAt) {
		return nil
	}
	for i, vessel := range s.vessels {
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
		if err := s.record(vessel); err != nil {
			return err
		}
		s.vessels[i] = vessel
	}
	s.updatedAt = now.UTC()
	return nil
}

// record is called with the state lock held; history is ordered oldest first.
func (s *Simulator) record(vessel Vessel) error {
	sentence, err := ais.EncodePosition(vessel.Position)
	if err != nil {
		return fmt.Errorf("encode vessel %d: %w", vessel.MMSI, err)
	}
	if len(s.messages) == MessageLimit {
		copy(s.messages, s.messages[1:])
		s.messages = s.messages[:MessageLimit-1]
	}
	s.messages = append(s.messages, Message{MMSI: vessel.MMSI, Timestamp: vessel.UpdatedAt, Sentence: sentence})
	return nil
}

// Snapshot returns a copy of the active fleet and current history size.
func (s *Simulator) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{Vessels: slices.Clone(s.vessels), MessageCount: len(s.messages), MessageLimit: MessageLimit, UpdatedAt: s.updatedAt}
}

// Messages returns a copy of the retained reports, ordered oldest first.
func (s *Simulator) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.messages)
}

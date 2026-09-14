package simulation_test

import (
	"math"
	"sync"
	"testing"
	"time"

	nmea "github.com/adrianmo/go-nmea"
	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/ais"
	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

const runID = "run-1"

var start = time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

func newSimulator(t *testing.T, seed uint64) *simulation.Simulator {
	t.Helper()
	s, err := simulation.New(runID, start, seed)
	require.NoError(t, err)
	return s
}

// decode returns the navigation data of a vessel's latest report.
func decode(t *testing.T, vessel simulation.Vessel) ais.Report {
	t.Helper()
	report, err := ais.DecodePosition(vessel.Report.Sentence)
	require.NoError(t, err)
	require.Equal(t, vessel.MMSI, report.MMSI)
	return report
}

func TestFleetLifecycle(t *testing.T) {
	now := start
	s := newSimulator(t, 42)
	initial := s.Fleet()
	require.Len(t, initial.Vessels, 1)
	require.Equal(t, 1, initial.MessageCount)
	vessel := initial.Vessels[0]
	origin := decode(t, vessel)
	require.NoError(t, s.SetCount(3, now))
	fleet := s.Fleet().Vessels
	require.Len(t, fleet, 3)
	require.Equal(t, vessel, fleet[0])
	require.NotEqual(t, fleet[0].MMSI, fleet[1].MMSI)
	require.NotEqual(t, fleet[1].MMSI, fleet[2].MMSI)
	require.NoError(t, s.Advance(now.Add(time.Minute)))
	movedVessel := s.Fleet().Vessels[0]
	moved := decode(t, movedVessel)
	require.NotEqual(t, *origin.Latitude, *moved.Latitude)
	// The great-circle distance must agree with one minute at reported speed,
	// within AIS coordinate precision.
	rad := math.Pi / 180
	dlat := (*moved.Latitude - *origin.Latitude) * rad
	dlon := (*moved.Longitude - *origin.Longitude) * rad
	a := math.Pow(math.Sin(dlat/2), 2) + math.Cos(*origin.Latitude*rad)*math.Cos(*moved.Latitude*rad)*math.Pow(math.Sin(dlon/2), 2)
	meters := 6371000 * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	require.InDelta(t, *origin.Speed*1852/60, meters, 0.5)
	require.Equal(t, now.Add(time.Minute), movedVessel.Report.Timestamp)
	require.Equal(t, 6, s.Fleet().MessageCount)
	require.NoError(t, s.Advance(now))
	require.Equal(t, 6, s.Fleet().MessageCount)
	require.NoError(t, s.SetCount(0, now.Add(time.Minute)))
	require.Empty(t, s.Fleet().Vessels)
	require.NotNil(t, s.Fleet().Vessels)
	require.NoError(t, s.Advance(now.Add(2*time.Minute)))
	require.Equal(t, now.Add(2*time.Minute), s.Fleet().UpdatedAt, "ticks advance time for an empty fleet")
	require.Len(t, s.History().Messages, 6)
	require.NoError(t, s.SetCount(1, now.Add(2*time.Minute)))
	require.NotEqual(t, vessel.MMSI, s.Fleet().Vessels[0].MMSI)
	before := s.Fleet()
	for _, count := range []int{-1, simulation.MaxVessels + 1} {
		require.Error(t, s.SetCount(count, now))
		require.Equal(t, before, s.Fleet())
	}
}

func TestReportsDescribeActiveFleet(t *testing.T) {
	s := newSimulator(t, 7)
	initial := s.Fleet()
	require.Equal(t, runID, initial.SimulationID)
	require.Equal(t, start, initial.UpdatedAt)
	require.Len(t, initial.Vessels, 1)
	first := initial.Vessels[0]
	require.Equal(t, "cargo", first.TypeID)
	require.Equal(t, uint64(1), first.Report.Sequence)
	require.Equal(t, start, first.Report.Timestamp)
	requireValidNMEA(t, first.Report.Sentence)
	requireBounds(t, s.History(), 1, 1)

	// Growing keeps existing identities and initializes new reports immediately.
	grown := start.Add(time.Second)
	require.NoError(t, s.SetCount(3, grown))
	fleet := s.Fleet()
	require.Equal(t, grown, fleet.UpdatedAt)
	require.Equal(t, first, fleet.Vessels[0])
	for i, vessel := range fleet.Vessels[1:] {
		require.Equal(t, uint64(i+2), vessel.Report.Sequence)
		require.Equal(t, grown, vessel.Report.Timestamp)
		requireValidNMEA(t, vessel.Report.Sentence)
	}

	// Reads never consume sequences.
	s.Fleet()
	s.History()
	s.Metadata()

	// Movement publishes one sentence to both latest report and history.
	moved := start.Add(2 * time.Second)
	require.NoError(t, s.Advance(moved))
	fleet = s.Fleet()
	history := s.History()
	requireBounds(t, history, 1, 6)
	for i, vessel := range fleet.Vessels {
		require.Equal(t, uint64(i+4), vessel.Report.Sequence)
		require.Equal(t, moved, vessel.Report.Timestamp)
		require.Equal(t, simulation.Message{
			Sequence: vessel.Report.Sequence, MMSI: vessel.MMSI,
			Timestamp: vessel.Report.Timestamp, Sentence: vessel.Report.Sentence,
		}, history.Messages[i+3])
	}
	require.NotEqual(t, first.Report.Sentence, fleet.Vessels[0].Report.Sentence)

	// Reducing removes active state only; history keeps the removed reports.
	reduced := start.Add(3 * time.Second)
	require.NoError(t, s.SetCount(1, reduced))
	fleet = s.Fleet()
	require.Len(t, fleet.Vessels, 1)
	require.Equal(t, first.MMSI, fleet.Vessels[0].MMSI)
	require.Equal(t, reduced, fleet.UpdatedAt)
	require.Equal(t, 6, fleet.MessageCount)
	requireBounds(t, s.History(), 1, 6)

	// A no-op count change keeps the update time.
	require.NoError(t, s.SetCount(1, start.Add(4*time.Second)))
	require.Equal(t, reduced, s.Fleet().UpdatedAt)

	require.NoError(t, s.SetCount(0, start.Add(5*time.Second)))
	require.NotNil(t, s.Fleet().Vessels, "an empty fleet is a non-nil slice")
	require.Empty(t, s.Fleet().Vessels)
}

func TestHistoryRetention(t *testing.T) {
	s := newSimulator(t, 1)
	require.NoError(t, s.SetCount(3, start))
	for i := 1; i <= 400; i++ {
		require.NoError(t, s.Advance(start.Add(time.Duration(i)*time.Second)))
	}
	// 3 initial reports plus 3 per tick.
	history := s.History()
	require.Len(t, history.Messages, simulation.MessageLimit)
	requireBounds(t, history, 204, 1203)
	for i, message := range history.Messages {
		require.Equal(t, uint64(204+i), message.Sequence)
	}
	for _, vessel := range s.Fleet().Vessels {
		message := history.Messages[vessel.Report.Sequence-204]
		require.Equal(t, vessel.MMSI, message.MMSI)
		require.Equal(t, vessel.Report.Sentence, message.Sentence)
	}
	require.Equal(t, simulation.MessageLimit, s.Fleet().MessageCount)
}

func TestReadsReturnCopies(t *testing.T) {
	s := newSimulator(t, 1)
	fleet := s.Fleet()
	fleet.Vessels[0].Name = "changed"
	fleet.Vessels[0].Report.Sentence = "changed"
	history := s.History()
	history.Messages[0].Sentence = "changed"
	*history.OldestSequence = 99
	*history.LatestSequence = 99
	metadata := s.Metadata()
	metadata.VesselTypes[0].Name = "changed"
	metadata.SupportedMessageTypes[0] = 99

	require.NotEqual(t, fleet, s.Fleet())
	require.NotEqual(t, history, s.History())
	require.NotEqual(t, metadata, s.Metadata())
	requireBounds(t, s.History(), 1, 1)
}

func TestMetadataDescribesGeneration(t *testing.T) {
	s := newSimulator(t, 3)
	metadata := s.Metadata()
	require.Equal(t, simulation.Metadata{
		SimulationID:          runID,
		StartedAt:             start,
		VesselTypes:           []simulation.VesselType{{ID: "cargo", Name: "Cargo vessel"}},
		SupportedMessageTypes: []int{1},
		Settings: simulation.Settings{
			InitialVesselCount: 1, MaxVessels: 100, TickIntervalMs: 1000, MessageIntervalMs: 1000, MessageHistoryLimit: 1000,
			SpeedKnots:  simulation.SpeedRange{Min: 6, Max: 15.9},
			SpawnBounds: simulation.SpawnBounds{South: 52, North: 52.04, West: 3.94, East: 4},
		},
	}, metadata)
	require.Len(t, s.Fleet().Vessels, metadata.Settings.InitialVesselCount)
	require.Equal(t, simulation.TickInterval.Milliseconds(), metadata.Settings.TickIntervalMs)

	require.NoError(t, s.SetCount(metadata.Settings.MaxVessels, start))
	require.Error(t, s.SetCount(metadata.Settings.MaxVessels+1, start))
	speed, bounds := metadata.Settings.SpeedKnots, metadata.Settings.SpawnBounds
	// Decoded coordinates are rounded to AIS precision, so the exclusive north
	// and east limits are checked inclusively.
	for _, vessel := range s.Fleet().Vessels {
		report := decode(t, vessel)
		require.GreaterOrEqual(t, *report.Latitude, bounds.South)
		require.LessOrEqual(t, *report.Latitude, bounds.North)
		require.GreaterOrEqual(t, *report.Longitude, bounds.West)
		require.LessOrEqual(t, *report.Longitude, bounds.East)
		require.GreaterOrEqual(t, *report.Speed, speed.Min)
		require.LessOrEqual(t, *report.Speed, speed.Max)
		require.Contains(t, metadata.VesselTypes, simulation.VesselType{ID: vessel.TypeID, Name: "Cargo vessel"})
	}
}

func TestSeedReproducesFleet(t *testing.T) {
	a := newSimulator(t, 42)
	b := newSimulator(t, 42)
	require.Equal(t, a.Fleet(), b.Fleet())
	require.Equal(t, a.History(), b.History())
}

func TestRunIdentity(t *testing.T) {
	_, err := simulation.New("", start, 1)
	require.Error(t, err)

	s := newSimulator(t, 42)
	require.Equal(t, runID, s.Metadata().SimulationID)
	require.Equal(t, runID, s.Fleet().SimulationID)
	require.Equal(t, runID, s.History().SimulationID)
}

func TestConcurrentAccess(t *testing.T) {
	s := newSimulator(t, 1)
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			for i := range 25 {
				if err := s.SetCount(i, start); err != nil {
					t.Error(err)
				}
				if err := s.Advance(start.Add(time.Duration(i) * time.Second)); err != nil {
					t.Error(err)
				}
				s.Fleet()
				s.History()
				s.Metadata()
			}
		})
	}
	group.Wait()
}

func requireValidNMEA(t *testing.T, sentence string) {
	t.Helper()
	require.Regexp(t, `^!AIVDM,.*\r\n$`, sentence)
	_, err := nmea.Parse(sentence)
	require.NoError(t, err)
}

func requireBounds(t *testing.T, history simulation.History, oldest, latest uint64) {
	t.Helper()
	require.NotNil(t, history.OldestSequence)
	require.NotNil(t, history.LatestSequence)
	require.Equal(t, oldest, *history.OldestSequence)
	require.Equal(t, latest, *history.LatestSequence)
	require.Equal(t, history.Messages[0].Sequence, oldest)
	require.Equal(t, history.Messages[len(history.Messages)-1].Sequence, latest)
}

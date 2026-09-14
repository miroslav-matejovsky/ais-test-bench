package simulation_test

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	nmea "github.com/adrianmo/go-nmea"
	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/ais"
	"github.com/miroslav-matejovsky/ais-testbench/simulation"
)

const runID = "run-1"

var start = time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

func testConfig(seed uint64, count int) simulation.Config {
	return simulation.Config{ID: runID, StartTime: start, Seed: seed, InitialVesselCount: count, Speed: 1, Transmitter: transmitter}
}

func newSimulator(t *testing.T, config simulation.Config) *simulation.Simulator {
	t.Helper()
	s, err := simulation.New(config)
	require.NoError(t, err)
	return s
}

// step is one ordered engine command returning the reports it emitted.
type step func(context.Context, *simulation.Simulator) ([]simulation.Message, error)

func advance(d time.Duration) step {
	return func(ctx context.Context, s *simulation.Simulator) ([]simulation.Message, error) {
		return s.Advance(ctx, d)
	}
}

func elapse(d time.Duration) step {
	return func(ctx context.Context, s *simulation.Simulator) ([]simulation.Message, error) {
		return s.Elapse(ctx, d)
	}
}

func setCount(count int) step {
	return func(_ context.Context, s *simulation.Simulator) ([]simulation.Message, error) {
		return s.SetCount(count)
	}
}

func setSpeed(speed float64) step {
	return func(_ context.Context, s *simulation.Simulator) ([]simulation.Message, error) {
		return []simulation.Message{}, s.SetSpeed(speed)
	}
}

// runSteps applies steps in order and returns all emitted reports.
func runSteps(t *testing.T, s *simulation.Simulator, steps ...step) []simulation.Message {
	t.Helper()
	all := make([]simulation.Message, 0)
	for _, step := range steps {
		reports, err := step(t.Context(), s)
		require.NoError(t, err)
		require.NotNil(t, reports)
		all = append(all, reports...)
	}
	return all
}

// snapshot is every observable read of an engine.
type snapshot struct {
	Fleet        simulation.Fleet
	History      simulation.History
	Metadata     simulation.Metadata
	Stations     simulation.StationSet
	Observations simulation.Observations
	Receptions   map[string]simulation.ReceptionPage // Complete retained history by station.
}

func observe(t *testing.T, s *simulation.Simulator) snapshot {
	t.Helper()
	observations, err := s.Observations(nil)
	require.NoError(t, err)
	receptions := map[string]simulation.ReceptionPage{}
	for _, id := range observations.Selection {
		page, err := s.ReceptionHistory(id, nil, simulation.ReceptionHistoryLimit)
		require.NoError(t, err)
		receptions[id] = page
	}
	return snapshot{
		Fleet: s.Fleet(), History: s.History(), Metadata: s.Metadata(), Stations: s.Stations(),
		Observations: observations, Receptions: receptions,
	}
}

// observeOutcome is observe without the state revision, which counts commits
// and so differs between split and combined calls.
func observeOutcome(t *testing.T, s *simulation.Simulator) snapshot {
	t.Helper()
	result := observe(t, s)
	result.Observations.StateRevision = 0
	return result
}

// decode returns the navigation data of an AIS sentence.
func decode(t *testing.T, sentence string) ais.Report {
	t.Helper()
	report, err := ais.DecodePosition(sentence)
	require.NoError(t, err)
	return report
}

// distance returns the great-circle distance between two reports in meters.
func distance(a, b ais.Report) float64 {
	rad := math.Pi / 180
	dlat := (*b.Latitude - *a.Latitude) * rad
	dlon := (*b.Longitude - *a.Longitude) * rad
	h := math.Pow(math.Sin(dlat/2), 2) + math.Cos(*a.Latitude*rad)*math.Cos(*b.Latitude*rad)*math.Pow(math.Sin(dlon/2), 2)
	return 6371000 * 2 * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	for name, change := range map[string]func(*simulation.Config){
		"empty id":         func(c *simulation.Config) { c.ID = "" },
		"zero start":       func(c *simulation.Config) { c.StartTime = time.Time{} },
		"start before 1":   func(c *simulation.Config) { c.StartTime = time.Date(0, 12, 31, 23, 0, 0, 0, time.UTC) },
		"start after 9999": func(c *simulation.Config) { c.StartTime = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
		"UTC after 9999": func(c *simulation.Config) {
			c.StartTime = time.Date(9999, 12, 31, 23, 30, 0, 0, time.FixedZone("west", -3600))
		},
		"negative count":      func(c *simulation.Config) { c.InitialVesselCount = -1 },
		"count over limit":    func(c *simulation.Config) { c.InitialVesselCount = simulation.MaxVessels + 1 },
		"negative speed":      func(c *simulation.Config) { c.Speed = -0.01 },
		"not a number":        func(c *simulation.Config) { c.Speed = math.NaN() },
		"infinity":            func(c *simulation.Config) { c.Speed = math.Inf(1) },
		"negative infinity":   func(c *simulation.Config) { c.Speed = math.Inf(-1) },
		"speed over limit":    func(c *simulation.Config) { c.Speed = simulation.MaxSpeed + simulation.SpeedStep },
		"below speed step":    func(c *simulation.Config) { c.Speed = 0.001 },
		"tiny speed":          func(c *simulation.Config) { c.Speed = 1e-12 },
		"between speed steps": func(c *simulation.Config) { c.Speed = 0.015 },
	} {
		t.Run(name, func(t *testing.T) {
			config := testConfig(1, 1)
			change(&config)
			s, err := simulation.New(config)
			require.ErrorIs(t, err, simulation.ErrInvalid)
			require.Nil(t, s)
		})
	}
}

func TestNewAcceptsExplicitValues(t *testing.T) {
	tenth, twentyNine := 0.1, 0.29
	tests := []struct {
		name   string
		change func(*simulation.Config)
		count  int
		speed  float64
	}{
		{name: "one vessel", change: func(*simulation.Config) {}, count: 1, speed: 1},
		{name: "zero count", change: func(c *simulation.Config) { c.InitialVesselCount = 0 }, count: 0, speed: 1},
		{name: "max count", change: func(c *simulation.Config) { c.InitialVesselCount = simulation.MaxVessels }, count: simulation.MaxVessels, speed: 1},
		{name: "zero seed", change: func(c *simulation.Config) { c.Seed = 0 }, count: 1, speed: 1},
		{name: "paused", change: func(c *simulation.Config) { c.Speed = 0 }, count: 1, speed: 0},
		{name: "min speed", change: func(c *simulation.Config) { c.Speed = simulation.MinSpeed }, count: 1, speed: 0.01},
		{name: "half speed", change: func(c *simulation.Config) { c.Speed = 0.5 }, count: 1, speed: 0.5},
		{name: "double speed", change: func(c *simulation.Config) { c.Speed = 2 }, count: 1, speed: 2},
		{name: "max speed", change: func(c *simulation.Config) { c.Speed = simulation.MaxSpeed }, count: 1, speed: 100},
		{name: "float sum", change: func(c *simulation.Config) { c.Speed = tenth + 0.2 }, count: 1, speed: 0.3},
		{name: "float product", change: func(c *simulation.Config) { c.Speed = twentyNine * 3 }, count: 1, speed: 0.87},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := testConfig(1, 1)
			tt.change(&config)
			s := newSimulator(t, config)
			metadata := s.Metadata()
			require.Equal(t, simulation.TimeState{Now: start, Speed: tt.speed, Paused: tt.speed == 0}, metadata.Time)
			require.Equal(t, tt.count, metadata.Settings.InitialVesselCount)
			require.Len(t, s.Fleet().Vessels, tt.count)
			require.Len(t, s.History().Messages, tt.count)
		})
	}
}

func TestNewNormalizesStartTime(t *testing.T) {
	for _, startTime := range []time.Time{
		start.In(time.FixedZone("CEST", 2*3600)),
		time.Date(1, 1, 1, 0, 0, 0, 1, time.UTC),
		time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC),
	} {
		config := testConfig(1, 1)
		config.StartTime = startTime
		s := newSimulator(t, config)
		metadata := s.Metadata()
		require.Equal(t, startTime.UTC(), metadata.StartedAt)
		require.Equal(t, time.UTC, metadata.StartedAt.Location())
		require.Equal(t, startTime.UTC(), metadata.Time.Now)
		require.Equal(t, startTime.UTC(), s.Fleet().Vessels[0].Report.Timestamp)
	}
}

func TestFleetLifecycle(t *testing.T) {
	s := newSimulator(t, testConfig(42, 1))
	initial := s.Fleet()
	require.Len(t, initial.Vessels, 1)
	require.Equal(t, 1, initial.MessageCount)
	vessel := initial.Vessels[0]
	origin := decode(t, vessel.Report.Sentence)
	runSteps(t, s, setCount(3))
	fleet := s.Fleet().Vessels
	require.Len(t, fleet, 3)
	require.Equal(t, vessel, fleet[0])
	require.NotEqual(t, fleet[0].MMSI, fleet[1].MMSI)
	require.NotEqual(t, fleet[1].MMSI, fleet[2].MMSI)

	require.Len(t, runSteps(t, s, advance(time.Minute)), 180)
	movedVessel := s.Fleet().Vessels[0]
	moved := decode(t, movedVessel.Report.Sentence)
	require.NotEqual(t, *origin.Latitude, *moved.Latitude)
	// Sixty ticks cover one minute at reported speed, within AIS coordinate
	// precision.
	require.InDelta(t, *origin.Speed*1852/60, distance(origin, moved), 0.5)
	require.Equal(t, start.Add(time.Minute), movedVessel.Report.Timestamp)
	require.Equal(t, 183, s.Fleet().MessageCount)

	runSteps(t, s, setCount(0))
	require.Empty(t, s.Fleet().Vessels)
	require.NotNil(t, s.Fleet().Vessels)
	require.Empty(t, runSteps(t, s, advance(time.Minute)))
	require.Equal(t, start.Add(2*time.Minute), s.Fleet().UpdatedAt, "ticks advance time for an empty fleet")
	require.Len(t, s.History().Messages, 183)
	runSteps(t, s, setCount(1))
	require.NotEqual(t, vessel.MMSI, s.Fleet().Vessels[0].MMSI)

	before := observe(t, s)
	for _, count := range []int{-1, simulation.MaxVessels + 1} {
		reports, err := s.SetCount(count)
		require.ErrorIs(t, err, simulation.ErrInvalid)
		require.Nil(t, reports)
		require.Equal(t, before, observe(t, s))
	}
}

func TestReportsDescribeActiveFleet(t *testing.T) {
	s := newSimulator(t, testConfig(7, 1))
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

	// Growing keeps existing identities and reports new vessels immediately at
	// the current virtual time.
	grown := start.Add(500 * time.Millisecond)
	created := runSteps(t, s, advance(500*time.Millisecond), setCount(3))
	fleet := s.Fleet()
	require.Equal(t, grown, fleet.UpdatedAt)
	require.Equal(t, first, fleet.Vessels[0])
	require.Len(t, created, 2)
	for i, vessel := range fleet.Vessels[1:] {
		require.Equal(t, uint64(i+2), vessel.Report.Sequence)
		require.Equal(t, grown, vessel.Report.Timestamp)
		require.Equal(t, simulation.Message{
			Sequence: vessel.Report.Sequence, MMSI: vessel.MMSI,
			Timestamp: vessel.Report.Timestamp, Sentence: vessel.Report.Sentence,
		}, created[i])
		requireValidNMEA(t, vessel.Report.Sentence)
	}

	// Reads never consume sequences.
	s.Fleet()
	s.History()
	s.Metadata()

	// A tick publishes one sentence per vessel to both latest report and history.
	moved := start.Add(time.Second)
	runSteps(t, s, advance(500*time.Millisecond))
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
	reduced := start.Add(1250 * time.Millisecond)
	require.Empty(t, runSteps(t, s, advance(250*time.Millisecond), setCount(1)))
	fleet = s.Fleet()
	require.Len(t, fleet.Vessels, 1)
	require.Equal(t, first.MMSI, fleet.Vessels[0].MMSI)
	require.Equal(t, reduced, fleet.UpdatedAt)
	require.Equal(t, 6, fleet.MessageCount)
	requireBounds(t, s.History(), 1, 6)

	// A no-op count change keeps the update time.
	require.Empty(t, runSteps(t, s, advance(250*time.Millisecond), setCount(1)))
	require.Equal(t, reduced, s.Fleet().UpdatedAt)

	runSteps(t, s, setCount(0))
	require.NotNil(t, s.Fleet().Vessels, "an empty fleet is a non-nil slice")
	require.Empty(t, s.Fleet().Vessels)
}

func TestTicksKeepExistingMovement(t *testing.T) {
	s := newSimulator(t, simulation.Config{
		ID: "test-run", StartTime: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC), Seed: 42, InitialVesselCount: 2, Speed: 1,
		Transmitter: transmitter,
	})
	runSteps(t, s, advance(time.Second))

	// Generation and movement at canonical ticks produce the sentences of the
	// engine before virtual time.
	sentences := make([]string, 0)
	for _, message := range s.History().Messages {
		sentences = append(sentences, fmt.Sprint(message.Sequence, " ", message.MMSI, " ", message.Timestamp.Format(time.RFC3339), " ", message.Sentence))
	}
	require.Equal(t, []string{
		"1 200000000 2030-01-02T03:04:05Z !AIVDM,1,1,,A,12vg200P0w0B9jjMiLDPe0T:0000,0*1D\r\n",
		"2 200000001 2030-01-02T03:04:05Z !AIVDM,1,1,,B,12vg20@P1n0B@wjMhDVo9Uf:0000,0*3D\r\n",
		"3 200000000 2030-01-02T03:04:06Z !AIVDM,1,1,,B,12vg200P0w0B9k4MiLHhe0T<0000,0*73\r\n",
		"4 200000001 2030-01-02T03:04:06Z !AIVDM,1,1,,A,12vg20@P1n0B@wdMhDNo9Uf<0000,0*2E\r\n",
	}, sentences)
}

func TestChannelsAlternatePerVessel(t *testing.T) {
	for _, count := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			s := newSimulator(t, testConfig(5, count))
			runSteps(t, s, advance(1500*time.Millisecond), setCount(count+1), advance(2500*time.Millisecond))

			last := map[uint32]ais.Channel{}
			for _, message := range s.History().Messages {
				want := ais.ChannelA
				if previous, seen := last[message.MMSI]; seen && previous == ais.ChannelA || !seen && message.MMSI%2 == 1 {
					want = ais.ChannelB
				}
				requireValidNMEA(t, message.Sentence)
				got := decode(t, message.Sentence).Channel
				require.Equal(t, want, got, "sequence %d", message.Sequence)
				last[message.MMSI] = got
			}
			require.Len(t, last, count+1)
		})
	}
}

func TestSpeedScalesElapsedTime(t *testing.T) {
	for _, tt := range []struct {
		speed   float64
		real    time.Duration
		virtual time.Duration
	}{
		{speed: 0, real: 10 * time.Second, virtual: 0},
		{speed: 0.01, real: 10 * time.Second, virtual: 100 * time.Millisecond},
		{speed: 0.5, real: 10 * time.Second, virtual: 5 * time.Second},
		{speed: 1, real: 10 * time.Second, virtual: 10 * time.Second},
		{speed: 2, real: 10 * time.Second, virtual: 20 * time.Second},
		{speed: 100, real: 600 * time.Millisecond, virtual: simulation.MaxAdvance},
	} {
		t.Run(fmt.Sprint(tt.speed), func(t *testing.T) {
			config := testConfig(4, 2)
			config.Speed = tt.speed
			s := newSimulator(t, config)
			knots := *decode(t, s.Fleet().Vessels[0].Report.Sentence).Speed

			reports := runSteps(t, s, elapse(tt.real))

			require.Len(t, reports, 2*int(tt.virtual/simulation.TickInterval))
			clock := s.Metadata().Time
			require.Equal(t, tt.virtual, clock.Elapsed)
			require.Equal(t, start.Add(tt.virtual), clock.Now)
			require.InDelta(t, knots, *decode(t, s.Fleet().Vessels[0].Report.Sentence).Speed, 0, "speed never changes reported knots")
		})
	}
}

func TestSetSpeedRejectsInvalidValues(t *testing.T) {
	s := newSimulator(t, testConfig(1, 1))
	before := observe(t, s)
	for _, speed := range []float64{-0.01, math.NaN(), math.Inf(1), math.Inf(-1), 100.01, 0.001, 1e-12, 0.015} {
		require.ErrorIs(t, s.SetSpeed(speed), simulation.ErrInvalid)
		require.ErrorIs(t, simulation.ValidateSpeed(speed), simulation.ErrInvalid)
	}
	require.Equal(t, before, observe(t, s))
}

func TestSplitCallsMatchCombined(t *testing.T) {
	ms := time.Millisecond
	config := testConfig(9, 3)
	config.Stations = receivers()
	groups := map[string][][]step{
		"ten seconds": {
			{advance(10 * time.Second)},
			slices.Repeat([]step{advance(time.Second)}, 10),
			{advance(300 * ms), advance(700 * ms), advance(0), advance(2500 * ms), advance(6500 * ms)},
			{advance(time.Second - 1), advance(1), advance(9 * time.Second)},
			{elapse(10 * time.Second)},
			{elapse(4 * time.Second), advance(0), elapse(6 * time.Second)},
			{setSpeed(0.5), elapse(20 * time.Second), setSpeed(1)},
			{setSpeed(100), elapse(30 * ms), elapse(70 * ms), setSpeed(1)},
			{advance(4 * time.Second), setSpeed(0), elapse(time.Hour), advance(time.Second), setSpeed(1), elapse(5 * time.Second)},
		},
		"count change at five seconds": {
			{advance(5 * time.Second), setCount(5), advance(5 * time.Second)},
			{advance(2 * time.Second), elapse(3 * time.Second), setCount(5), elapse(2500 * ms), advance(2500 * ms)},
			{setSpeed(2), elapse(1250 * ms), setSpeed(1), advance(2500 * ms), setCount(5), setSpeed(0.25), elapse(20 * time.Second), setSpeed(1)},
		},
	}
	for name, scenarios := range groups {
		t.Run(name, func(t *testing.T) {
			reference := newSimulator(t, config)
			want := runSteps(t, reference, scenarios[0]...)
			for i, steps := range scenarios[1:] {
				s := newSimulator(t, config)
				require.Equal(t, want, runSteps(t, s, steps...), "scenario %d", i+1)
				require.Equal(t, observeOutcome(t, reference), observeOutcome(t, s), "scenario %d", i+1)
			}
			require.Equal(t, start.Add(10*time.Second), reference.Metadata().Time.Now)
			marginal := observe(t, reference).Observations.Stations[1].Counters
			require.NotZero(t, marginal.Received, "the marginal station receives")
			require.NotZero(t, marginal.ProbabilisticLoss+marginal.OutsideHorizon, "the marginal station misses")
		})
	}
}

func TestElapseCarriesSubNanosecondRemainder(t *testing.T) {
	config := testConfig(2, 2)
	config.Speed = 0.33
	config.Stations = receivers()
	scenarios := [][]step{
		{elapse(10*time.Second + 3), elapse(1), setSpeed(2), elapse(time.Second), advance(time.Second), setSpeed(0.01), elapse(68)},
		{
			elapse(1), elapse(1), elapse(1), elapse(10 * time.Second), elapse(1), setSpeed(2),
			elapse(500 * time.Millisecond), advance(time.Second), elapse(500 * time.Millisecond),
			setSpeed(0.01), elapse(34), elapse(34),
		},
	}
	reference := newSimulator(t, config)
	want := runSteps(t, reference, scenarios[0]...)
	// 3.3s from 0.33x plus 2ns built only from carried hundredths, 2s at 2x, and
	// one explicit second. Without carrying, the final 68ns at 0.01x add nothing.
	require.Equal(t, 6300*time.Millisecond+2, reference.Metadata().Time.Elapsed)
	for _, steps := range scenarios[1:] {
		s := newSimulator(t, config)
		require.Equal(t, want, runSteps(t, s, steps...))
		require.Equal(t, observeOutcome(t, reference), observeOutcome(t, s))
	}
}

func TestPauseDiscardsElapsedTime(t *testing.T) {
	s := newSimulator(t, testConfig(5, 2))
	before := observe(t, s)
	require.NoError(t, s.SetSpeed(1))
	require.Equal(t, before, observe(t, s), "a repeated speed is a no-op")
	require.NoError(t, s.SetSpeed(0))
	require.Equal(t, before.History, s.History(), "setting speed emits no reports")
	require.Equal(t, simulation.TimeState{Now: start, Speed: 0, Paused: true}, s.Metadata().Time)

	require.Empty(t, runSteps(t, s, elapse(time.Hour)))
	require.Equal(t, start, s.Metadata().Time.Now)

	require.Len(t, runSteps(t, s, advance(2*time.Second)), 4, "explicit steps work while paused")
	require.Equal(t, start.Add(2*time.Second), s.Metadata().Time.Now)

	require.NoError(t, s.SetSpeed(1))
	reports := runSteps(t, s, elapse(time.Second))
	require.Len(t, reports, 2, "resuming does not catch up paused time")
	require.Equal(t, start.Add(3*time.Second), reports[1].Timestamp)
}

// TestCountChangesUseVirtualTime is the regression test for count changes with
// caller timestamps, which could emit reports older than the fleet and suppress
// a tick that was due.
func TestCountChangesUseVirtualTime(t *testing.T) {
	s := newSimulator(t, testConfig(3, 1))
	runSteps(t, s, advance(10*time.Second))

	created := runSteps(t, s, setCount(2))
	require.Len(t, created, 1)
	require.Equal(t, uint64(12), created[0].Sequence)
	require.Equal(t, start.Add(10*time.Second), created[0].Timestamp)
	require.Equal(t, start.Add(10*time.Second), s.Fleet().UpdatedAt)

	due := runSteps(t, s, advance(500*time.Millisecond), setCount(3), advance(500*time.Millisecond))
	require.Len(t, due, 4, "a count change between ticks keeps the next tick")
	require.Equal(t, start.Add(10500*time.Millisecond), due[0].Timestamp)
	for _, report := range due[1:] {
		require.Equal(t, start.Add(11*time.Second), report.Timestamp)
	}

	history := s.History().Messages
	for i := 1; i < len(history); i++ {
		require.Greater(t, history[i].Sequence, history[i-1].Sequence)
		require.False(t, history[i].Timestamp.Before(history[i-1].Timestamp))
	}
}

func TestNewVesselsMoveOnlyFromBirth(t *testing.T) {
	control := newSimulator(t, testConfig(3, 1))
	runSteps(t, control, advance(2*time.Second))

	s := newSimulator(t, testConfig(3, 1))
	between := runSteps(t, s, advance(500*time.Millisecond), setCount(2))
	boundary := runSteps(t, s, advance(500*time.Millisecond), setCount(3))
	runSteps(t, s, advance(time.Second))

	fleet := s.Fleet().Vessels
	want := control.Fleet().Vessels[0].Report
	require.Equal(t, want.Timestamp, fleet[0].Report.Timestamp)
	require.Equal(t, want.Sentence, fleet[0].Report.Sentence, "new vessels do not move existing vessels")

	// The boundary result holds the first vessel's tick, then the creation.
	require.Len(t, boundary, 3)
	require.Equal(t, start.Add(time.Second), boundary[2].Timestamp)
	for _, tt := range []struct {
		birth  simulation.Message
		vessel simulation.Vessel
		moved  time.Duration
	}{
		{birth: between[0], vessel: fleet[1], moved: 1500 * time.Millisecond},
		{birth: boundary[2], vessel: fleet[2], moved: time.Second},
	} {
		require.Equal(t, tt.birth.MMSI, tt.vessel.MMSI)
		require.Equal(t, start.Add(2*time.Second), tt.vessel.Report.Timestamp)
		born, moved := decode(t, tt.birth.Sentence), decode(t, tt.vessel.Report.Sentence)
		require.InDelta(t, *born.Speed*1852*tt.moved.Hours(), distance(born, moved), 0.5)
	}
}

func TestEmptyFleetKeepsClock(t *testing.T) {
	s := newSimulator(t, testConfig(1, 0))
	history := s.History()
	require.Empty(t, history.Messages)
	require.Nil(t, history.OldestSequence)

	require.Empty(t, runSteps(t, s, advance(2500*time.Millisecond)))
	require.Equal(t, start.Add(2500*time.Millisecond), s.Metadata().Time.Now)
	require.Equal(t, start.Add(2*time.Second), s.Fleet().UpdatedAt)

	created := runSteps(t, s, setCount(1))
	require.Len(t, created, 1)
	require.Equal(t, start.Add(2500*time.Millisecond), created[0].Timestamp)
	reports := runSteps(t, s, advance(500*time.Millisecond))
	require.Len(t, reports, 1)
	require.Equal(t, start.Add(3*time.Second), reports[0].Timestamp)

	// Removing at a boundary happens after that tick.
	require.Empty(t, runSteps(t, s, setCount(0), advance(time.Second)))
	require.Equal(t, start.Add(4*time.Second), s.Fleet().UpdatedAt)
	requireBounds(t, s.History(), 1, 2)
}

func TestFailedAdvancesChangeNothing(t *testing.T) {
	s := newSimulator(t, testConfig(1, 1))
	before := observe(t, s)
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	for _, tt := range []struct {
		name string
		call func() ([]simulation.Message, error)
		want error
	}{
		{name: "negative advance", call: func() ([]simulation.Message, error) { return s.Advance(t.Context(), -1) }, want: simulation.ErrInvalid},
		{name: "negative elapse", call: func() ([]simulation.Message, error) { return s.Elapse(t.Context(), -1) }, want: simulation.ErrInvalid},
		{name: "advance over limit", call: func() ([]simulation.Message, error) { return s.Advance(t.Context(), simulation.MaxAdvance+1) }, want: simulation.ErrLimit},
		{name: "elapse over limit", call: func() ([]simulation.Message, error) { return s.Elapse(t.Context(), simulation.MaxAdvance+1) }, want: simulation.ErrLimit},
		{name: "cancelled advance", call: func() ([]simulation.Message, error) { return s.Advance(cancelled, time.Second) }, want: context.Canceled},
		{name: "cancelled zero advance", call: func() ([]simulation.Message, error) { return s.Advance(cancelled, 0) }, want: context.Canceled},
		{name: "cancelled elapse", call: func() ([]simulation.Message, error) { return s.Elapse(cancelled, time.Second) }, want: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reports, err := tt.call()
			require.ErrorIs(t, err, tt.want)
			require.Nil(t, reports)
			require.Equal(t, before, observe(t, s))
		})
	}

	require.NoError(t, s.SetSpeed(simulation.MaxSpeed))
	before = observe(t, s)
	for _, realDelta := range []time.Duration{simulation.MaxAdvance/100 + 1, math.MaxInt64} {
		reports, err := s.Elapse(t.Context(), realDelta)
		require.ErrorIs(t, err, simulation.ErrLimit)
		require.Nil(t, reports)
	}
	require.Equal(t, before, observe(t, s))
	require.NoError(t, s.SetSpeed(1))

	require.Empty(t, runSteps(t, s, advance(0)))
	require.Len(t, runSteps(t, s, advance(simulation.MaxAdvance)), 60)
	require.Len(t, runSteps(t, s, elapse(simulation.MaxAdvance)), 60)
}

// cancelAfter reports cancellation after Err returned nil checks times.
type cancelAfter struct {
	context.Context
	checks int
}

func (c *cancelAfter) Err() error {
	if c.checks == 0 {
		return context.Canceled
	}
	c.checks--
	return nil
}

func TestCancellationBetweenTicksRollsBack(t *testing.T) {
	config := testConfig(6, 3)
	config.Speed = 0.33
	config.Stations = receivers()
	s, control := newSimulator(t, config), newSimulator(t, config)
	// Both engines carry a scaling remainder into the cancelled call.
	runSteps(t, s, elapse(3))
	runSteps(t, control, elapse(3))
	before := observe(t, s)

	// 30s at 0.33x covers nine ticks; the fourth tick sees the cancellation.
	reports, err := s.Elapse(&cancelAfter{Context: t.Context(), checks: 3}, 30*time.Second)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, reports)
	require.Equal(t, before, observe(t, s))

	steps := []step{elapse(30 * time.Second), elapse(1)}
	require.Equal(t, runSteps(t, control, steps...), runSteps(t, s, steps...))
	require.Equal(t, observe(t, control), observe(t, s))
}

func TestTimestampsCrossCalendarBoundaries(t *testing.T) {
	config := testConfig(2, 1)
	config.StartTime = time.Date(2030, 1, 1, 0, 59, 57, 250000000, time.FixedZone("CET", 3600))
	s := newSimulator(t, config)
	runSteps(t, s, advance(3*time.Second))

	messages := s.History().Messages
	require.Len(t, messages, 4)
	for i, want := range []string{"2029-12-31T23:59:57.25Z", "2029-12-31T23:59:58.25Z", "2029-12-31T23:59:59.25Z", "2030-01-01T00:00:00.25Z"} {
		require.Equal(t, want, messages[i].Timestamp.Format(time.RFC3339Nano))
		require.Equal(t, time.UTC, messages[i].Timestamp.Location())
		require.Equal(t, messages[i].Timestamp.Second(), *decode(t, messages[i].Sentence).Second, "AIS second matches the report time")
	}

	config.StartTime = time.Date(9999, 12, 31, 23, 59, 30, 0, time.UTC)
	late := newSimulator(t, config)
	before := observe(t, late)
	reports, err := late.Advance(t.Context(), 30*time.Second)
	require.ErrorIs(t, err, simulation.ErrLimit)
	require.Nil(t, reports)
	require.Equal(t, before, observe(t, late))
	require.Len(t, runSteps(t, late, advance(30*time.Second-1)), 29)
	require.Equal(t, time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC), late.Metadata().Time.Now)
	_, err = late.Elapse(t.Context(), 1)
	require.ErrorIs(t, err, simulation.ErrLimit)
}

func TestAdvanceReturnsCompleteBatch(t *testing.T) {
	s := newSimulator(t, testConfig(8, simulation.MaxVessels))

	reports := runSteps(t, s, advance(simulation.MaxAdvance))

	require.Len(t, reports, 6000)
	fleet := s.Fleet().Vessels
	for i, report := range reports {
		require.Equal(t, uint64(simulation.MaxVessels+1+i), report.Sequence)
		require.Equal(t, fleet[i%simulation.MaxVessels].MMSI, report.MMSI, "each tick reports in creation order")
		require.Equal(t, start.Add(time.Duration(i/simulation.MaxVessels+1)*time.Second), report.Timestamp)
	}
	history := s.History()
	require.Equal(t, reports[len(reports)-simulation.MessageLimit:], history.Messages)
	reports[len(reports)-1].Sentence = "changed"
	require.Equal(t, history, s.History(), "returned reports are detached")
}

func TestHistoryRetention(t *testing.T) {
	s := newSimulator(t, testConfig(1, 3))
	for range 400 {
		runSteps(t, s, advance(time.Second))
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
	s := newSimulator(t, testConfig(1, 1))
	created := runSteps(t, s, setCount(2))
	created[0].Sentence = "changed"
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
	metadata.Settings.Reception.CoverageThresholds[0] = 99
	require.InDelta(t, 0.9, s.Metadata().Settings.Reception.CoverageThresholds[0], 0)

	require.NotEqual(t, fleet, s.Fleet())
	require.NotEqual(t, history, s.History())
	require.NotEqual(t, metadata, s.Metadata())
	require.NotEqual(t, created[0].Sentence, s.History().Messages[1].Sentence)
	requireBounds(t, s.History(), 1, 2)
}

func TestMetadataDescribesGeneration(t *testing.T) {
	s := newSimulator(t, testConfig(3, 1))
	metadata := s.Metadata()
	require.Equal(t, simulation.Metadata{
		SimulationID:          runID,
		StartedAt:             start,
		Time:                  simulation.TimeState{Now: start, Speed: 1},
		VesselTypes:           []simulation.VesselType{{ID: "cargo", Name: "Cargo vessel"}},
		SupportedMessageTypes: []int{1},
		Settings: simulation.Settings{
			InitialVesselCount: 1, MaxVessels: 100, TickIntervalMs: 1000, MessageIntervalMs: 1000, MessageHistoryLimit: 1000,
			SpeedKnots:  simulation.SpeedRange{Min: 6, Max: 15.9},
			SpawnBounds: simulation.SpawnBounds{South: 52, North: 52.04, West: 3.94, East: 4},
			Speed:       simulation.SpeedLimits{Min: 0.01, Max: 100, Step: 0.01},
			MaxStations: 16,
			Transmitter: simulation.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1},
			Reception: simulation.ReceptionModel{
				SiteLossDB: 15, PathExponent: 3.5, EffectiveEarthRadiusFactor: 4.0 / 3,
				ChannelAFrequencyMHz: 161.975, ChannelBFrequencyMHz: 162.025, HorizonTaperStart: 0.8,
				ZeroProbabilityMarginDB: -12, ReferenceProbability: 0.8, FullProbabilityMarginDB: 6,
				CoverageThresholds: []float64{0.9, 0.5},
			},
			Observation: simulation.ObservationSettings{
				ReceptionHistoryLimit: 1000, TargetLimit: 1000, RecentReceptionLimit: 50,
				FreshAgeMs: 10000, StaleAgeMs: 60000, ExpiryAgeMs: 600000, RateWindowMs: 60000,
			},
		},
	}, metadata)
	require.Len(t, s.Fleet().Vessels, metadata.Settings.InitialVesselCount)
	require.Equal(t, simulation.TickInterval.Milliseconds(), metadata.Settings.TickIntervalMs)

	runSteps(t, s, setCount(metadata.Settings.MaxVessels))
	_, err := s.SetCount(metadata.Settings.MaxVessels + 1)
	require.ErrorIs(t, err, simulation.ErrInvalid)
	speed, bounds := metadata.Settings.SpeedKnots, metadata.Settings.SpawnBounds
	// Decoded coordinates are rounded to AIS precision, so the exclusive north
	// and east limits are checked inclusively.
	for _, vessel := range s.Fleet().Vessels {
		report := decode(t, vessel.Report.Sentence)
		require.Equal(t, vessel.MMSI, report.MMSI)
		require.GreaterOrEqual(t, *report.Latitude, bounds.South)
		require.LessOrEqual(t, *report.Latitude, bounds.North)
		require.GreaterOrEqual(t, *report.Longitude, bounds.West)
		require.LessOrEqual(t, *report.Longitude, bounds.East)
		require.GreaterOrEqual(t, *report.Speed, speed.Min)
		require.LessOrEqual(t, *report.Speed, speed.Max)
		require.Contains(t, metadata.VesselTypes, simulation.VesselType{ID: vessel.TypeID, Name: "Cargo vessel"})
	}
}

func TestSeedReproducesRun(t *testing.T) {
	ms := time.Millisecond
	steps := []step{setCount(4), advance(2500 * ms), setSpeed(2), elapse(1500 * ms), setCount(1), advance(ms)}
	for _, seed := range []uint64{0, 42} {
		a, b := newSimulator(t, testConfig(seed, 2)), newSimulator(t, testConfig(seed, 2))
		require.Equal(t, runSteps(t, a, steps...), runSteps(t, b, steps...))
		require.Equal(t, observe(t, a), observe(t, b))
	}
	require.NotEqual(t, newSimulator(t, testConfig(0, 2)).Fleet(), newSimulator(t, testConfig(42, 2)).Fleet())
}

func TestRunIdentity(t *testing.T) {
	s := newSimulator(t, testConfig(42, 1))
	require.Equal(t, runID, s.Metadata().SimulationID)
	require.Equal(t, runID, s.Fleet().SimulationID)
	require.Equal(t, runID, s.History().SimulationID)
}

func TestConcurrentAccess(t *testing.T) {
	ms := time.Millisecond
	steps := []step{setCount(5), advance(1500 * ms), setSpeed(2), elapse(700 * ms), setCount(2), advance(3 * time.Second)}
	config := testConfig(1, 1)
	config.Stations = receivers()
	control, s := newSimulator(t, config), newSimulator(t, config)

	// Concurrent readers do not change serialized deterministic mutations.
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					s.Fleet()
					s.History()
					s.Metadata()
					if _, err := s.Observations(nil); err != nil {
						t.Error(err)
					}
				}
			}
		})
	}
	got := runSteps(t, s, steps...)
	close(stop)
	readers.Wait()
	require.Equal(t, runSteps(t, control, steps...), got)
	require.Equal(t, observe(t, control), observe(t, s))

	// Concurrent mutations are safe; their order is not deterministic.
	var writers sync.WaitGroup
	for i := range 4 {
		writers.Go(func() {
			for j := range 25 {
				if _, err := s.SetCount((i + j) % simulation.MaxVessels); err != nil {
					t.Error(err)
				}
				if _, err := s.Advance(t.Context(), 100*ms); err != nil {
					t.Error(err)
				}
				if err := s.SetSpeed(float64(j)); err != nil {
					t.Error(err)
				}
				if _, err := s.Elapse(t.Context(), 100*ms); err != nil {
					t.Error(err)
				}
				s.Fleet()
				s.History()
				s.Metadata()
			}
		})
	}
	writers.Wait()
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

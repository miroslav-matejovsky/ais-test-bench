package simdriver_test

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/ais-testbench/simulation"
)

var (
	virtualStart = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	realStart    = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
)

// fakeClock is a manual real clock. Its ticker channel is unbuffered, so a send
// completes only when Run receives the wakeup.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	ticks   chan time.Time
	stopped atomic.Bool
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: realStart, ticks: make(chan time.Time)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *fakeClock) NewTicker(time.Duration) (<-chan time.Time, func()) {
	return c.ticks, func() { c.stopped.Store(true) }
}

func newEngine(t *testing.T) *simulation.Simulator {
	t.Helper()
	sim, err := simulation.New(simulation.Config{
		ID: "run-1", StartTime: virtualStart, Seed: 1, InitialVesselCount: 1, Speed: 1,
		Transmitter: simulation.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1},
		Stations:    []simulation.StationDefinition{site("Site")},
	})
	require.NoError(t, err)
	return sim
}

func site(name string) simulation.StationDefinition {
	channel := simulation.ReceiverChannel{Enabled: true, SensitivityDBm: -110}
	return simulation.StationDefinition{
		Name: name, Latitude: 52, Longitude: 4, Enabled: true, AntennaHeightMeters: 25, ChannelA: channel, ChannelB: channel,
	}
}

func newDriver(t *testing.T) (*simulation.Simulator, *fakeClock, *simdriver.Driver) {
	t.Helper()
	sim, clock := newEngine(t), newFakeClock()
	return sim, clock, simdriver.NewDriver(sim, clock)
}

// run starts driver.Run. wake returns after Run processed one heartbeat: the
// second send waits until Run receives again. stop cancels Run and returns its
// result.
func run(t *testing.T, clock *fakeClock, driver *simdriver.Driver) (wake func(), stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- driver.Run(ctx) }()
	wake = func() {
		clock.ticks <- time.Time{}
		clock.ticks <- time.Time{}
	}
	stop = func() error {
		cancel()
		return <-done
	}
	return wake, stop
}

// observe returns every observable read of an engine.
func observe(sim *simulation.Simulator) [4]any {
	return [4]any{sim.Fleet(), sim.History(), sim.Metadata(), sim.Stations()}
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

func TestRunDeliversMeasuredElapsedTime(t *testing.T) {
	sim, clock, driver := newDriver(t)
	control := newEngine(t)
	wake, stop := run(t, clock, driver)

	clock.Add(simdriver.Heartbeat)
	wake()
	require.Equal(t, virtualStart.Add(simdriver.Heartbeat), sim.Metadata().Time.Now)

	// A late heartbeat delivers all elapsed time, including every due tick.
	clock.Add(2350 * time.Millisecond)
	wake()
	require.Equal(t, virtualStart.Add(2450*time.Millisecond), sim.Metadata().Time.Now)
	require.Len(t, sim.History().Messages, 3)

	// Coalesced wakeups lose no time.
	clock.Add(250 * time.Millisecond)
	clock.Add(250 * time.Millisecond)
	wake()
	require.NoError(t, stop())
	require.True(t, clock.stopped.Load(), "Run releases its ticker")

	for _, d := range []time.Duration{simdriver.Heartbeat, 2350 * time.Millisecond, 500 * time.Millisecond} {
		_, err := control.Elapse(t.Context(), d)
		require.NoError(t, err)
	}
	require.Equal(t, observe(control), observe(sim), "driver output equals a manually driven engine")
}

func TestCommandsSettleAtPreviousSpeed(t *testing.T) {
	sim, clock, driver := newDriver(t)

	clock.Add(1500 * time.Millisecond)
	require.NoError(t, driver.SetSpeed(t.Context(), 2))
	require.Equal(t, simulation.TimeState{
		Now: virtualStart.Add(1500 * time.Millisecond), Elapsed: 1500 * time.Millisecond, Speed: 2,
	}, sim.Metadata().Time)
	before := observe(sim)
	require.NoError(t, driver.SetSpeed(t.Context(), 2))
	require.Equal(t, before, observe(sim), "a repeated speed without elapsed time changes nothing")

	// 250ms at 2x reaches the tick at 2s exactly; the vessel is created after it.
	clock.Add(250 * time.Millisecond)
	require.NoError(t, driver.SetCount(t.Context(), 2))
	messages := sim.History().Messages
	require.Len(t, messages, 4)
	fleet := sim.Fleet().Vessels
	tick, created := messages[2], messages[3]
	require.Equal(t, fleet[0].MMSI, tick.MMSI)
	require.Equal(t, fleet[1].MMSI, created.MMSI)
	require.Equal(t, virtualStart.Add(2*time.Second), tick.Timestamp)
	require.Equal(t, virtualStart.Add(2*time.Second), created.Timestamp)
}

func TestInvalidCommandsDoNotSettle(t *testing.T) {
	sim, clock, driver := newDriver(t)
	clock.Add(5 * time.Second)
	before := observe(sim)

	require.ErrorIs(t, driver.SetCount(t.Context(), -1), simulation.ErrInvalid)
	require.ErrorIs(t, driver.SetCount(t.Context(), simulation.MaxVessels+1), simulation.ErrInvalid)
	require.ErrorIs(t, driver.SetSpeed(t.Context(), math.NaN()), simulation.ErrInvalid)
	require.ErrorIs(t, driver.SetSpeed(t.Context(), 0.015), simulation.ErrInvalid)
	require.Equal(t, before, observe(sim))

	require.NoError(t, driver.SetSpeed(t.Context(), 1))
	require.Equal(t, virtualStart.Add(5*time.Second), sim.Metadata().Time.Now, "a valid repeated speed settles")
}

func TestPauseDropsRealTime(t *testing.T) {
	sim, clock, driver := newDriver(t)
	wake, stop := run(t, clock, driver)

	clock.Add(time.Second)
	require.NoError(t, driver.SetSpeed(t.Context(), 0))
	paused := virtualStart.Add(time.Second)
	require.Equal(t, simulation.TimeState{Now: paused, Elapsed: time.Second, Paused: true}, sim.Metadata().Time)

	// Real time far beyond the catch-up limit passes while paused.
	clock.Add(3 * simdriver.MaxCatchUp)
	wake()
	require.NoError(t, driver.SetSpeed(t.Context(), 1))
	require.Equal(t, paused, sim.Metadata().Time.Now, "resuming does not catch up paused time")

	clock.Add(500 * time.Millisecond)
	wake()
	require.Equal(t, paused.Add(500*time.Millisecond), sim.Metadata().Time.Now)
	require.NoError(t, stop())
}

func TestClockRegressionKeepsBaseline(t *testing.T) {
	sim, clock, driver := newDriver(t)
	wake, stop := run(t, clock, driver)

	clock.Add(-time.Second)
	wake()
	require.Equal(t, virtualStart, sim.Metadata().Time.Now)

	clock.Add(1500 * time.Millisecond)
	wake()
	require.Equal(t, virtualStart.Add(500*time.Millisecond), sim.Metadata().Time.Now, "regressed samples neither rewind nor double-count")
	require.NoError(t, stop())
}

func TestCatchUpDeliversEveryTick(t *testing.T) {
	sim, clock, driver := newDriver(t)
	clock.Add(simdriver.MaxCatchUp)

	require.NoError(t, driver.SetCount(t.Context(), 1))

	require.Equal(t, virtualStart.Add(simdriver.MaxCatchUp), sim.Metadata().Time.Now)
	require.Equal(t, uint64(3601), sim.Fleet().Vessels[0].Report.Sequence, "the initial report and one per virtual second")
}

func TestBacklogOverLimitFails(t *testing.T) {
	for _, tt := range []struct {
		speed   float64
		backlog time.Duration
	}{
		{speed: 1, backlog: simdriver.MaxCatchUp + time.Millisecond},
		{speed: simulation.MaxSpeed, backlog: simdriver.MaxCatchUp/100 + time.Millisecond},
	} {
		t.Run(fmt.Sprint(tt.speed), func(t *testing.T) {
			sim, clock, driver := newDriver(t)
			require.NoError(t, driver.SetSpeed(t.Context(), tt.speed))
			before := observe(sim)
			clock.Add(tt.backlog)

			require.ErrorContains(t, driver.SetCount(t.Context(), 2), "catch-up limit")
			require.Equal(t, before, observe(sim), "an over-limit backlog fails before delivery")

			done := make(chan error, 1)
			go func() { done <- driver.Run(t.Context()) }()
			clock.ticks <- time.Time{}
			require.ErrorContains(t, <-done, "catch-up limit", "the failure stops Run")
			require.Equal(t, before, observe(sim))
		})
	}
}

func TestCancellationStopsBetweenChunks(t *testing.T) {
	sim, clock, driver := newDriver(t)
	clock.Add(3 * time.Second)

	// The driver first checks cancellation; the engine then checks before and
	// after each 600ms chunk at 1x,
	// which holds at most one tick.
	err := driver.SetCount(&cancelAfter{Context: t.Context(), checks: 5}, 2)

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, virtualStart.Add(1200*time.Millisecond), sim.Metadata().Time.Now, "complete chunks stay delivered")
	require.Len(t, sim.Fleet().Vessels, 1, "the count change is not applied")
	require.NoError(t, driver.SetCount(t.Context(), 2))
	require.Equal(t, virtualStart.Add(3*time.Second), sim.Metadata().Time.Now, "the rest is delivered once")
}

func TestRunStopsOnCancellationAndRunsOnce(t *testing.T) {
	_, clock, driver := newDriver(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.NoError(t, driver.Run(ctx))
	require.True(t, clock.stopped.Load())
	require.ErrorContains(t, driver.Run(t.Context()), "already started")
}

func TestDriverDelegatesToEngine(t *testing.T) {
	sim, _, driver := newDriver(t)

	require.NoError(t, driver.SetCount(t.Context(), 3))

	require.Len(t, driver.Fleet().Vessels, 3)
	require.Equal(t, sim.Fleet(), driver.Fleet())
	require.Equal(t, sim.History(), driver.History())
	require.Equal(t, sim.Metadata(), driver.Metadata())
	require.Equal(t, sim.StationConfiguration(), driver.Stations())
}

func TestStationCommandsSettleAtCurrentInstant(t *testing.T) {
	sim, clock, driver := newDriver(t)

	clock.Add(1500 * time.Millisecond)
	id, stations, err := driver.AddStation(t.Context(), "run-1", 1, site("Added"))
	require.NoError(t, err)
	require.Equal(t, "station-2", id)
	require.Equal(t, sim.StationConfiguration(), stations)
	require.Equal(t, virtualStart.Add(1500*time.Millisecond), stations.Stations[1].CreatedAt)
	require.Len(t, sim.History().Messages, 2, "the tick before the edit is delivered")

	clock.Add(time.Second)
	stations, err = driver.UpdateStation(t.Context(), "run-1", 2, id, site("Renamed"))
	require.NoError(t, err)
	require.Equal(t, uint64(3), stations.Revision)
	require.Equal(t, virtualStart.Add(2500*time.Millisecond), sim.Metadata().Time.Now)

	clock.Add(time.Second)
	stations, err = driver.RemoveStation(t.Context(), "run-1", 3, id)
	require.NoError(t, err)
	require.Equal(t, []simulation.Station{sim.Stations().Stations[0]}, stations.Stations)
	require.Equal(t, virtualStart.Add(3500*time.Millisecond), sim.Metadata().Time.Now)
}

func TestRejectedStationCommandsDoNotSettle(t *testing.T) {
	sim, clock, driver := newDriver(t)
	clock.Add(5 * time.Second)
	before := observe(sim)

	_, _, err := driver.AddStation(t.Context(), "run-1", 1, site(""))
	require.ErrorIs(t, err, simulation.ErrInvalid)
	_, _, err = driver.AddStation(t.Context(), "run-1", 2, site("Stale"))
	require.ErrorIs(t, err, simulation.ErrConflict)
	_, err = driver.UpdateStation(t.Context(), "run-1", 1, "station-1", site(""))
	require.ErrorIs(t, err, simulation.ErrInvalid)
	_, err = driver.UpdateStation(t.Context(), "run-1", 0, "station-1", site("Stale"))
	require.ErrorIs(t, err, simulation.ErrConflict)
	_, err = driver.UpdateStation(t.Context(), "run-1", 1, "station-9", site("Missing"))
	require.ErrorIs(t, err, simulation.ErrNotFound)
	_, err = driver.RemoveStation(t.Context(), "run-1", 2, "station-1")
	require.ErrorIs(t, err, simulation.ErrConflict)
	_, err = driver.RemoveStation(t.Context(), "run-1", 1, "station-9")
	require.ErrorIs(t, err, simulation.ErrNotFound)
	require.Equal(t, before, observe(sim))
}

func TestStationRunIdentityAndCancelledPause(t *testing.T) {
	sim, clock, driver := newDriver(t)
	clock.Add(time.Second)
	before := sim.StationConfiguration()
	_, _, err := driver.AddStation(t.Context(), "old-run", 1, site("New"))
	require.ErrorIs(t, err, simulation.ErrConflict)
	_, err = driver.RemoveStation(t.Context(), "old-run", 1, "station-1")
	require.ErrorIs(t, err, simulation.ErrConflict)
	require.Equal(t, before, sim.StationConfiguration())
	require.NoError(t, driver.SetSpeed(t.Context(), 0))
	before = sim.StationConfiguration()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = driver.AddStation(ctx, "run-1", 1, site("Cancelled"))
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, before, sim.StationConfiguration())
}

func TestNewConfigStartsFreshRealTimeRun(t *testing.T) {
	before := time.Now()
	a, b := simdriver.NewConfig(), simdriver.NewConfig()

	require.NotEmpty(t, a.ID)
	require.NotEqual(t, a.ID, b.ID)
	require.False(t, a.StartTime.Before(before))
	require.Equal(t, 1, a.InitialVesselCount)
	require.InDelta(t, 1.0, a.Speed, 0)
	require.Equal(t, simulation.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1}, a.Transmitter)
	sim, err := simulation.New(a)
	require.NoError(t, err)

	stations := sim.Stations().Stations
	require.Len(t, stations, 3)
	for i, want := range []struct {
		name        string
		lat, lon    float64
		height      float64
		sensitivity float64
		sectors     int
	}{
		{name: "Rotterdam coast", lat: 51.98, lon: 4.05, height: 25, sensitivity: -110},
		{name: "Northern coast", lat: 52.12, lon: 4.24, height: 40, sensitivity: -112},
		{name: "Harbour receiver", lat: 51.95, lon: 4.14, height: 15, sensitivity: -108, sectors: 1},
	} {
		got := stations[i].Definition
		require.Equal(t, want.name, got.Name)
		require.InDelta(t, want.lat, got.Latitude, 0)
		require.InDelta(t, want.lon, got.Longitude, 0)
		require.InDelta(t, want.height, got.AntennaHeightMeters, 0)
		require.True(t, got.Enabled)
		for _, channel := range []simulation.ReceiverChannel{got.ChannelA, got.ChannelB} {
			require.Equal(t, simulation.ReceiverChannel{Enabled: true, SensitivityDBm: want.sensitivity}, channel)
		}
		require.Len(t, got.ShadowSectors, want.sectors)
	}
	require.Equal(t, []simulation.ShadowSector{{StartDegrees: 270, EndDegrees: 330, LossDB: 15}}, stations[2].Definition.ShadowSectors)
}

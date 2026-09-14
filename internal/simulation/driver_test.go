package simulation_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	simdriver "github.com/miroslav-matejovsky/ais-test-bench/internal/simulation"
	"github.com/miroslav-matejovsky/ais-test-bench/simulation"
)

func newDriver(t *testing.T) (*simulation.Simulator, *simdriver.Driver) {
	t.Helper()
	sim, err := simulation.New(simulation.Config{
		ID: "run-1", StartTime: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC), Seed: 1, InitialVesselCount: 1, Speed: 1,
	})
	require.NoError(t, err)
	return sim, simdriver.NewDriver(sim)
}

func TestRunStopsOnCancellationAndRunsOnce(t *testing.T) {
	_, driver := newDriver(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.NoError(t, driver.Run(ctx))
	require.ErrorContains(t, driver.Run(ctx), "already started")
}

func TestDriverDelegatesToEngine(t *testing.T) {
	sim, driver := newDriver(t)

	require.NoError(t, driver.SetCount(3))

	fleet := driver.Fleet()
	require.Len(t, fleet.Vessels, 3)
	require.Equal(t, sim.Fleet(), fleet)
	require.Equal(t, sim.History(), driver.History())
	require.Equal(t, sim.Metadata(), driver.Metadata())
	require.ErrorIs(t, driver.SetCount(simulation.MaxVessels+1), simulation.ErrInvalid)
	require.Equal(t, fleet, driver.Fleet())
}

func TestNewConfigStartsFreshRealTimeRun(t *testing.T) {
	before := time.Now()
	a, b := simdriver.NewConfig(), simdriver.NewConfig()

	require.NotEmpty(t, a.ID)
	require.NotEqual(t, a.ID, b.ID)
	require.False(t, a.StartTime.Before(before))
	require.Equal(t, 1, a.InitialVesselCount)
	require.InDelta(t, 1.0, a.Speed, 0)
	_, err := simulation.New(a)
	require.NoError(t, err)
}

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
	sim, err := simulation.New("run-1", time.Now(), 1)
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
	before := time.Now().UTC()

	require.NoError(t, driver.SetCount(3))

	fleet := driver.Fleet()
	require.Len(t, fleet.Vessels, 3)
	require.False(t, fleet.UpdatedAt.Before(before), "count changes use the current real time")
	require.Equal(t, sim.Fleet(), fleet)
	require.Equal(t, sim.History(), driver.History())
	require.Equal(t, sim.Metadata(), driver.Metadata())
	require.Error(t, driver.SetCount(simulation.MaxVessels+1))
	require.Equal(t, fleet, driver.Fleet())
}

func TestNewIDIsUnique(t *testing.T) {
	a, b := simdriver.NewID(), simdriver.NewID()
	require.NotEmpty(t, a)
	require.NotEqual(t, a, b)
}

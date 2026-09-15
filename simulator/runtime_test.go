package simulator

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/display"
	"github.com/miroslav-matejovsky/ais-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/ais-testbench/simulation"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

// runtimeClock gives tests explicit startup and heartbeat synchronization.
type runtimeClock struct {
	testClock
	started chan struct{}
	ticks   chan time.Time
	stopped chan struct{}
}

func (c *runtimeClock) NewTicker(time.Duration) (<-chan time.Time, func()) {
	close(c.started)
	return c.ticks, func() { close(c.stopped) }
}

func runtimeConfig() Config {
	return Config{Simulation: simulation.Config{
		ID: "local-run", StartTime: start, Seed: 3, InitialVesselCount: 3, Speed: 1,
		Transmitter: simulation.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1},
		Stations:    scenarioStations(),
	}}
}

func TestRuntimeConstructionAndTermination(t *testing.T) {
	_, err := New(Config{})
	require.ErrorIs(t, err, simulation.ErrInvalid)
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "catch-up failure"}[failure], func(t *testing.T) {
			clock := &runtimeClock{testClock: testClock{now: start}, started: make(chan struct{}), ticks: make(chan time.Time), stopped: make(chan struct{})}
			sim, err := newSimulator(runtimeConfig(), clock)
			require.NoError(t, err)
			select {
			case <-clock.started:
				t.Fatal("constructor started pacing")
			default:
			}
			require.Equal(t, start, sim.Metadata().Time.Now)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- sim.Run(ctx) }()
			<-clock.started
			require.ErrorContains(t, sim.Run(t.Context()), "already started")
			// A rejected second Run must not stop the first run's commands.
			require.NoError(t, sim.SetCount(t.Context(), 2))
			if failure {
				clock.Add(simdriver.MaxCatchUp + time.Second)
				clock.ticks <- time.Time{}
				require.ErrorContains(t, <-done, "catch-up limit")
			} else {
				cancel()
				require.NoError(t, <-done)
			}
			<-clock.stopped
			before := sim.Stations()
			require.ErrorIs(t, sim.SetCount(t.Context(), 4), simulatorapi.ErrUnavailable)
			require.ErrorIs(t, sim.SetSpeed(t.Context(), 2), simulatorapi.ErrUnavailable)
			_, _, err = sim.AddStation(t.Context(), before.SimulationID, before.Revision, scenarioStations()[0])
			require.ErrorIs(t, err, simulatorapi.ErrUnavailable)
			_, err = sim.UpdateStation(t.Context(), before.SimulationID, before.Revision, coast, scenarioStations()[0])
			require.ErrorIs(t, err, simulatorapi.ErrUnavailable)
			_, err = sim.RemoveStation(t.Context(), before.SimulationID, before.Revision, coast)
			require.ErrorIs(t, err, simulatorapi.ErrUnavailable)
			require.Equal(t, before, sim.Stations())
			_, err = sim.Observations(t.Context(), nil)
			require.NoError(t, err, "terminal snapshots remain readable")
			require.Equal(t, 503, serve(sim.API(), "PUT", "/vessels", `{"count":1}`).Code)
			require.ErrorContains(t, sim.Run(t.Context()), "already started")
		})
	}
}

func TestCancelledCommandsDoNotChangePausedOrUnadvancedRuntime(t *testing.T) {
	for _, speed := range []float64{0, 1} {
		config := runtimeConfig()
		config.Simulation.Speed = speed
		sim, err := newSimulator(config, &testClock{now: start})
		require.NoError(t, err)
		before := sim.Fleet()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, sim.SetCount(ctx, 0), context.Canceled)
		require.ErrorIs(t, sim.SetSpeed(ctx, 2), context.Canceled)
		require.Equal(t, before, sim.Fleet())
		require.Equal(t, speed, sim.Metadata().Time.Speed)
	}
}

func TestLocalAndRemoteSourceParity(t *testing.T) {
	sim, err := newSimulator(runtimeConfig(), &testClock{now: start})
	require.NoError(t, err)
	server := httptest.NewServer(sim.API())
	t.Cleanup(server.Close)
	remote, err := display.NewHTTPSource(display.HTTPConfig{APIBase: server.URL + "/"})
	require.NoError(t, err)
	t.Cleanup(remote.CloseIdleConnections)
	localClient, err := display.New(sim)
	require.NoError(t, err)
	remoteClient, err := display.New(remote)
	require.NoError(t, err)
	for _, selection := range [][]string{nil, {coast}, {coast, harbour}, {deaf}} {
		local, err := localClient.Observations(t.Context(), selection)
		require.NoError(t, err)
		network, err := remoteClient.Observations(t.Context(), selection)
		require.NoError(t, err)
		require.Equal(t, local, network)
		localJSON, err := json.Marshal(local)
		require.NoError(t, err)
		networkJSON, err := json.Marshal(network)
		require.NoError(t, err)
		require.JSONEq(t, string(localJSON), string(networkJSON))
		require.Contains(t, string(localJSON), `"stateRevision":"1"`)
	}
	req := simulatorapi.HistoryRequest{SimulationID: "local-run"}
	local, err := localClient.ReceptionHistory(t.Context(), coast, req)
	require.NoError(t, err)
	network, err := remoteClient.ReceptionHistory(t.Context(), coast, req)
	require.NoError(t, err)
	require.Equal(t, local, network)
	require.NotEmpty(t, local.Receptions)
	for _, r := range local.Receptions {
		require.Contains(t, r.Sentence, "\r\n")
	}
	for _, source := range []display.Source{sim, remote} {
		_, err := source.Observations(t.Context(), []string{"unknown"})
		require.ErrorIs(t, err, simulatorapi.ErrNotFound)
		_, err = source.Observations(t.Context(), []string{coast, coast})
		require.ErrorIs(t, err, simulatorapi.ErrInvalidRequest)
		_, err = source.ReceptionHistory(t.Context(), "unknown", req)
		require.ErrorIs(t, err, simulatorapi.ErrNotFound)
		_, err = source.ReceptionHistory(t.Context(), "unknown", simulatorapi.HistoryRequest{SimulationID: "previous"})
		require.ErrorIs(t, err, simulatorapi.ErrConflict, "run is checked before station lookup")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = source.Observations(ctx, nil)
		require.ErrorIs(t, err, context.Canceled)
		require.ErrorIs(t, err, simulatorapi.ErrUnavailable)
		_, err = source.ReceptionHistory(ctx, coast, req)
		require.ErrorIs(t, err, context.Canceled)
	}
	// Mutation of returned wire data cannot change subsequent local snapshots.
	first, err := sim.Observations(t.Context(), nil)
	require.NoError(t, err)
	first.Stations[0].Definition.Name = "changed by caller"
	second, err := sim.Observations(t.Context(), nil)
	require.NoError(t, err)
	require.NotEqual(t, first.Stations[0].Definition.Name, second.Stations[0].Definition.Name)
}

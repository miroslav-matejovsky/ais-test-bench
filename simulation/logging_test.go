package simulation_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/logtest"
	"github.com/miroslav-matejovsky/ais-testbench/simulation"
)

// TestLoggerDoesNotChangeResults runs one scenario with no logger, an enabled
// logger, and a logger with every level disabled.
func TestLoggerDoesNotChangeResults(t *testing.T) {
	enabled := logtest.New(slog.LevelDebug, nil)
	disabled := logtest.New(slog.LevelError+1, nil)
	defaultLogger := slog.Default()
	run := func(logger *slog.Logger) []any {
		t.Helper()
		channel := simulation.ReceiverChannel{Enabled: true, SensitivityDBm: -110}
		sim, err := simulation.New(simulation.Config{
			ID: "logging", StartTime: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), Seed: 7, InitialVesselCount: 5, Speed: 1,
			Transmitter: simulation.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1},
			Stations: []simulation.StationDefinition{{
				Name: "coast", Latitude: 51.98, Longitude: 4.05, Enabled: true,
				AntennaHeightMeters: 25, ReceiveGainDBi: 3, FeederLossDB: 2, ChannelA: channel, ChannelB: channel,
			}},
			Logger: logger,
		})
		require.NoError(t, err)
		reports, err := sim.Advance(t.Context(), 30*time.Second)
		require.NoError(t, err)
		created, err := sim.SetCount(8)
		require.NoError(t, err)
		elapsed, err := sim.Elapse(t.Context(), 1500*time.Millisecond)
		require.NoError(t, err)
		observations, err := sim.Observations(nil)
		require.NoError(t, err)
		return []any{reports, created, elapsed, sim.Fleet(), sim.History(), sim.Metadata(), observations}
	}

	want := run(nil)
	require.Equal(t, want, run(slog.New(enabled)))
	require.Equal(t, want, run(slog.New(disabled)))
	require.Empty(t, enabled.Records(), "the engine emits no log records")
	require.Empty(t, disabled.Records())
	require.Same(t, defaultLogger, slog.Default(), "construction never replaces the default logger")
}

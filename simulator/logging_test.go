package simulator

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/logtest"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

type contextKey struct{}

// hostLogger returns a recorder and a host logger with an attribute and a group,
// like an embedding application would supply.
func hostLogger(level slog.Level, hook func(context.Context, logtest.Record)) (*logtest.Recorder, *slog.Logger) {
	recorder := logtest.New(level, hook)
	return recorder, slog.New(recorder).With("app", "embedder").WithGroup("bench")
}

// stoppedSimulator returns a simulator whose Run has ended, so commands fail
// with simulatorapi.ErrUnavailable and the API logs them.
func stoppedSimulator(t *testing.T, config Config) *Simulator {
	t.Helper()
	sim, err := newSimulator(config, &testClock{now: start})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, sim.Run(ctx))
	return sim
}

// putCount sends a count change to path on handler.
func putCount(ctx context.Context, handler http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(ctx, http.MethodPut, path, strings.NewReader(`{"count":2}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAPIFailureLogsToConfiguredLogger(t *testing.T) {
	var sim *Simulator
	recorder, logger := hostLogger(slog.LevelInfo, func(context.Context, logtest.Record) {
		// Both calls take the driver and engine locks, so they would deadlock if
		// the record were emitted while either lock is held.
		require.ErrorIs(t, sim.SetSpeed(context.Background(), 1), simulatorapi.ErrUnavailable)
		require.NotEmpty(t, sim.Fleet().Vessels)
	})
	config := runtimeConfig()
	config.Logger = logger
	sim = stoppedSimulator(t, config)

	ctx := context.WithValue(t.Context(), contextKey{}, "request-1")
	require.Equal(t, http.StatusServiceUnavailable, putCount(ctx, sim.API(), "/vessels").Code)

	records := recorder.Records()
	require.Len(t, records, 1)
	record := records[0]
	require.Equal(t, slog.LevelError, record.Level)
	require.Equal(t, "set vessel count", record.Message)
	require.Equal(t, "embedder", record.Attrs["app"])
	require.Equal(t, "simulator", record.Attrs["bench.component"])
	require.Equal(t, "/vessels", record.Attrs["bench.path"])
	require.EqualValues(t, 2, record.Attrs["bench.count"])
	require.ErrorIs(t, record.Attrs["bench.error"].(error), simulatorapi.ErrUnavailable)
	require.Equal(t, "request-1", record.Context.Value(contextKey{}))
}

func TestIndependentSimulatorsUseTheirOwnLoggers(t *testing.T) {
	first, firstLogger := hostLogger(slog.LevelInfo, nil)
	second, secondLogger := hostLogger(slog.LevelInfo, nil)
	firstConfig, secondConfig := runtimeConfig(), runtimeConfig()
	firstConfig.Logger, secondConfig.Logger = firstLogger, secondLogger
	firstSim, secondSim := stoppedSimulator(t, firstConfig), stoppedSimulator(t, secondConfig)
	secondHandler, err := NewStandaloneHandler(secondSim, "")
	require.NoError(t, err)

	putCount(t.Context(), firstSim.API(), "/vessels")
	require.Len(t, first.Records(), 1)
	require.Empty(t, second.Records())

	putCount(t.Context(), secondHandler, "/api/vessels")
	require.Len(t, first.Records(), 1)
	require.Len(t, second.Records(), 1)
}

func TestLoggerPrecedenceAndDefault(t *testing.T) {
	engine, engineLogger := hostLogger(slog.LevelInfo, nil)
	config := runtimeConfig()
	config.Simulation.Logger = engineLogger
	putCount(t.Context(), stoppedSimulator(t, config).API(), "/vessels")
	require.Len(t, engine.Records(), 1, "nil Config.Logger falls back to Simulation.Logger")

	own, ownLogger := hostLogger(slog.LevelInfo, nil)
	config.Logger = ownLogger
	putCount(t.Context(), stoppedSimulator(t, config).API(), "/vessels")
	require.Len(t, own.Records(), 1, "Config.Logger takes precedence")
	require.Len(t, engine.Records(), 1)

	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	fallback := logtest.New(slog.LevelInfo, nil)
	defaultLogger := slog.New(fallback)
	slog.SetDefault(defaultLogger)
	sim := stoppedSimulator(t, runtimeConfig())
	require.Same(t, defaultLogger, slog.Default(), "construction never replaces the default logger")
	putCount(t.Context(), sim.API(), "/vessels")
	records := fallback.Records()
	require.Len(t, records, 1)
	require.Equal(t, "simulator", records[0].Attrs["component"])
}

func TestDisabledLogLevelsDoNotChangeResponses(t *testing.T) {
	enabled, enabledLogger := hostLogger(slog.LevelDebug, nil)
	disabled, disabledLogger := hostLogger(slog.LevelError+1, nil)
	var responses [2][2]string
	for i, logger := range []*slog.Logger{enabledLogger, disabledLogger} {
		config := runtimeConfig()
		config.Logger = logger
		api := stoppedSimulator(t, config).API()
		failed := putCount(t.Context(), api, "/vessels")
		read := serve(api, http.MethodGet, "/vessels", "")
		responses[i] = [2]string{failed.Result().Status + failed.Body.String(), read.Result().Status + read.Body.String()}
	}
	require.Equal(t, responses[0], responses[1])
	require.Len(t, enabled.Records(), 1)
	require.Empty(t, disabled.Records())
}

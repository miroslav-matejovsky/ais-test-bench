package display_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/display"
	"github.com/miroslav-matejovsky/ais-testbench/internal/logtest"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

type contextKey struct{}

// brokenSource returns responses that violate the source contract.
type brokenSource struct{}

func (brokenSource) Observations(context.Context, []string) (simulatorapi.Observations, error) {
	return simulatorapi.Observations{}, fmt.Errorf("%w: broken observations", simulatorapi.ErrInvalidResponse)
}

func (brokenSource) ReceptionHistory(context.Context, string, simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
	return simulatorapi.ReceptionPage{}, fmt.Errorf("%w: broken history", simulatorapi.ErrInvalidResponse)
}

func hostLogger(level slog.Level) (*logtest.Recorder, *slog.Logger) {
	recorder := logtest.New(level, nil)
	return recorder, slog.New(recorder).With("app", "embedder").WithGroup("bench")
}

func brokenHandlers(t *testing.T, logger *slog.Logger) map[string]http.Handler {
	t.Helper()
	client, err := display.New(brokenSource{})
	require.NoError(t, err)
	api, err := display.NewHandler(display.Config{Client: client, Logger: logger})
	require.NoError(t, err)
	standalone, err := display.NewStandaloneHandler(display.StandaloneConfig{Client: client, Logger: logger})
	require.NoError(t, err)
	return map[string]http.Handler{"NewHandler": http.StripPrefix("/display/api", api), "NewStandaloneHandler": standalone}
}

func readObservations(ctx context.Context, handler http.Handler) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/display/api/observations", nil))
	return rec
}

func TestSourceFailureLogsToSuppliedLogger(t *testing.T) {
	for name := range brokenHandlers(t, slog.New(slog.DiscardHandler)) {
		t.Run(name, func(t *testing.T) {
			recorder, logger := hostLogger(slog.LevelInfo)
			ctx := context.WithValue(t.Context(), contextKey{}, "request-1")
			require.Equal(t, http.StatusBadGateway, readObservations(ctx, brokenHandlers(t, logger)[name]).Code)

			records := recorder.Records()
			require.Len(t, records, 1)
			record := records[0]
			require.Equal(t, slog.LevelWarn, record.Level)
			require.Equal(t, "read simulator observations", record.Message)
			require.Equal(t, "embedder", record.Attrs["app"])
			require.Equal(t, "display", record.Attrs["bench.component"])
			require.Equal(t, "/observations", record.Attrs["bench.path"], "handlers log their local route path")
			require.EqualValues(t, http.StatusBadGateway, record.Attrs["bench.status"])
			require.ErrorIs(t, record.Attrs["bench.error"].(error), simulatorapi.ErrInvalidResponse)
			require.Equal(t, "request-1", record.Context.Value(contextKey{}))
		})
	}
}

func TestIndependentDisplayHandlersUseTheirOwnLoggers(t *testing.T) {
	first, firstLogger := hostLogger(slog.LevelInfo)
	second, secondLogger := hostLogger(slog.LevelInfo)
	firstHandlers, secondHandlers := brokenHandlers(t, firstLogger), brokenHandlers(t, secondLogger)

	readObservations(t.Context(), firstHandlers["NewHandler"])
	readObservations(t.Context(), firstHandlers["NewStandaloneHandler"])
	require.Len(t, first.Records(), 2)
	require.Empty(t, second.Records())

	readObservations(t.Context(), secondHandlers["NewHandler"])
	require.Len(t, first.Records(), 2)
	require.Len(t, second.Records(), 1)
}

func TestNilLoggerUsesDefaultAndLevelsDoNotChangeResponses(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	fallback := logtest.New(slog.LevelInfo, nil)
	defaultLogger := slog.New(fallback)
	slog.SetDefault(defaultLogger)

	nilHandlers := brokenHandlers(t, nil)
	require.Same(t, defaultLogger, slog.Default(), "construction never replaces the default logger")
	disabled, disabledLogger := hostLogger(slog.LevelError + 1)
	disabledHandlers := brokenHandlers(t, disabledLogger)
	for name, handler := range nilHandlers {
		logged := readObservations(t.Context(), handler)
		quiet := readObservations(t.Context(), disabledHandlers[name])
		require.Equal(t, logged.Code, quiet.Code)
		require.Equal(t, logged.Body.String(), quiet.Body.String())
	}
	records := fallback.Records()
	require.Len(t, records, 2)
	require.Equal(t, "display", records[0].Attrs["component"])
	require.Empty(t, disabled.Records())
}

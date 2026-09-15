package httpserver_test

import (
	"log/slog"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/httpserver"
	"github.com/miroslav-matejovsky/ais-testbench/internal/logtest"
)

func TestServerErrorsKeepLoggerAttributesAndGroups(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	recorder := logtest.New(slog.LevelInfo, nil)
	logger := slog.New(recorder).With("app", "host").WithGroup("bench").With("component", "simulator")
	server := httpserver.Serve(logger, ln, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("handler failed")
	}))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String(), nil)
	require.NoError(t, err)
	// net/http logs the recovered panic before closing the connection.
	_, err = http.DefaultClient.Do(req)
	require.Error(t, err)
	require.NoError(t, server.Shutdown(t.Context()))

	records := recorder.Records()
	require.NotEmpty(t, records)
	record := records[0]
	require.Equal(t, slog.LevelError, record.Level)
	require.Contains(t, record.Message, "handler failed")
	require.Equal(t, "host", record.Attrs["app"])
	require.Equal(t, "simulator", record.Attrs["bench.component"])
}

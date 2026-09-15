package testbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/display"
	"github.com/miroslav-matejovsky/ais-testbench/internal/logtest"
	"github.com/miroslav-matejovsky/ais-testbench/simulation"
	"github.com/miroslav-matejovsky/ais-testbench/simulator"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
	"github.com/miroslav-matejovsky/ais-testbench/testbench"
	"github.com/miroslav-matejovsky/ais-testbench/ui"
)

func config(basePath string) testbench.Config {
	return testbench.Config{Simulation: simulator.DemoConfig(), Logger: slog.New(slog.DiscardHandler), BasePath: basePath}
}

func do(handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var value T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &value))
	return value
}

func TestHandlerServesOneBenchBelowBasePath(t *testing.T) {
	for _, base := range []string{"", "/tools/ais"} {
		t.Run("base "+base, func(t *testing.T) {
			bench, err := testbench.New(config(base))
			require.NoError(t, err)
			h := bench.Handler()

			home := do(h, http.MethodGet, base+"/", "")
			require.Equal(t, http.StatusOK, home.Code)
			require.Contains(t, home.Body.String(), `href="`+base+`/manager"`)
			require.Contains(t, home.Body.String(), `href="`+base+`/display"`)
			manager := do(h, http.MethodGet, base+"/manager", "").Body.String()
			require.Contains(t, manager, "<h1>Manager</h1>")
			require.Contains(t, manager, `data-api-base="`+base+`/api/"`)
			require.Contains(t, manager, `hx-get="`+base+`/status"`)
			require.Contains(t, manager, `href="`+base+`/assets/css/ui.css"`)
			require.Contains(t, do(h, http.MethodGet, base+"/display", "").Body.String(), `data-api-base="`+base+`/display/api/"`)
			require.Equal(t, http.StatusOK, do(h, http.MethodGet, base+"/status", "").Code)
			require.Equal(t, http.StatusOK, do(h, http.MethodGet, base+"/assets/js/ui.js", "").Code)

			metadata := decode[simulatorapi.Metadata](t, do(h, http.MethodGet, base+"/api/metadata", ""))
			upstream := decode[simulatorapi.Observations](t, do(h, http.MethodGet, base+"/api/observations", ""))
			projection := decode[display.Observations](t, do(h, http.MethodGet, base+"/display/api/observations?stations=all", ""))
			require.Equal(t, metadata.SimulationID, projection.SimulationID, "the display reads the same engine")
			require.Len(t, projection.Stations, len(upstream.Stations))
			require.NotEmpty(t, projection.Stations, "demonstration stations are configured")
			first := projection.Stations[0].ID
			page := decode[display.ReceptionPage](t, do(h, http.MethodGet, base+"/display/api/stations/"+first+"/receptions?simulationId="+metadata.SimulationID, ""))
			require.Equal(t, first, page.StationID)

			require.Equal(t, http.StatusOK, do(h, http.MethodPut, base+"/api/vessels", `{"count":3}`).Code)
			require.Equal(t, http.StatusOK, do(h, http.MethodPut, base+"/api/vessels", `{"count":0}`).Code)
			history := decode[simulatorapi.History](t, do(h, http.MethodGet, base+"/api/messages", ""))
			for i, message := range history.Messages {
				require.Equal(t, uint64(i+1), message.Sequence, "one engine emits each report once")
			}
		})
	}
}

func TestHandlerKeepsPrefixAndLeavesOtherPaths(t *testing.T) {
	bench, err := testbench.New(config("/tools/ais"))
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "host") })
	mux.Handle("/tools/ais/", bench.Handler())

	redirect := do(mux, http.MethodGet, "/tools/ais/api?view=1", "")
	require.GreaterOrEqual(t, redirect.Code, http.StatusMovedPermanently)
	require.Less(t, redirect.Code, http.StatusBadRequest)
	require.Equal(t, "/tools/ais/api/?view=1", redirect.Header().Get("Location"))
	require.Equal(t, "/tools/ais/", do(mux, http.MethodGet, "/tools/ais", "").Header().Get("Location"))
	for _, path := range []string{"/", "/manager", "/api/metadata"} {
		require.Equal(t, "host", do(mux, http.MethodGet, path, "").Body.String(), path)
	}
	for _, path := range []string{"/manager", "/api/metadata", "/tools/ais/unknown", "/tools/ais/api/"} {
		require.Equal(t, http.StatusNotFound, do(bench.Handler(), http.MethodGet, path, "").Code, path)
	}
}

func TestNewValidatesConfig(t *testing.T) {
	for _, base := range []string{"/", "/tools/", "tools", "//tools", "/tools/../ais", "/tools%2Fais"} {
		_, err := testbench.New(config(base))
		require.ErrorContains(t, err, "BasePath", base)
	}
	_, err := testbench.New(testbench.Config{Logger: slog.New(slog.DiscardHandler)})
	require.ErrorIs(t, err, simulation.ErrInvalid)
	invalidTiles := config("")
	invalidTiles.Tiles = ui.MapTiles{URL: "javascript:alert(1)", Attribution: "x"}
	_, err = testbench.New(invalidTiles)
	require.ErrorContains(t, err, "Tiles.URL")
}

func TestLoggerReplacesEngineLogger(t *testing.T) {
	own := logtest.New(slog.LevelInfo, nil)
	engine := logtest.New(slog.LevelInfo, nil)
	cfg := config("")
	cfg.Logger = slog.New(own).With("app", "host")
	cfg.Simulation.Logger = slog.New(engine)
	bench, err := testbench.New(cfg)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, bench.Run(ctx))

	require.Equal(t, http.StatusServiceUnavailable, do(bench.Handler(), http.MethodPut, "/api/vessels", `{"count":2}`).Code)

	records := own.Records()
	require.Len(t, records, 1)
	require.Equal(t, "host", records[0].Attrs["app"])
	require.Equal(t, "simulator", records[0].Attrs["component"])
	require.Empty(t, engine.Records())
}

func TestRunLeavesHostServerRunning(t *testing.T) {
	bench, err := testbench.New(config("/tools/ais"))
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /host", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "host") })
	mux.Handle("/tools/ais/", bench.Handler())
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- bench.Run(ctx) }()

	status, _ := call(t, http.MethodPut, server.URL+"/tools/ais/api/vessels", `{"count":2}`)
	require.Equal(t, http.StatusOK, status)
	cancel()
	require.NoError(t, <-done)

	status, body := call(t, http.MethodGet, server.URL+"/host", "")
	require.Equal(t, http.StatusOK, status, "the host server keeps serving")
	require.Equal(t, "host", body)
	status, _ = call(t, http.MethodGet, server.URL+"/tools/ais/api/vessels", "")
	require.Equal(t, http.StatusOK, status, "reads work after Run")
	status, _ = call(t, http.MethodPut, server.URL+"/tools/ais/api/vessels", `{"count":1}`)
	require.Equal(t, http.StatusServiceUnavailable, status, "writes are unavailable after Run")
}

func call(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode, string(data)
}

// bodyListener accepts one connection and signals on waiting when the server
// reads that connection a second time. The client writes the request headers in
// one write and delays the body, so the second read is the handler waiting for
// the body of an active request. Close is signalled on closed.
type bodyListener struct {
	net.Listener
	waiting   chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
}

func newBodyListener(t *testing.T) *bodyListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return &bodyListener{Listener: ln, waiting: make(chan struct{}), closed: make(chan struct{})}
}

func (l *bodyListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &countingConn{Conn: conn, waiting: l.waiting}, nil
}

func (l *bodyListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return l.Listener.Close()
}

// countingConn closes waiting when Read is entered for the second time.
type countingConn struct {
	net.Conn
	reads   int // Only the connection's serving goroutine reads before the body arrives.
	waiting chan struct{}
}

func (c *countingConn) Read(p []byte) (int, error) {
	c.reads++
	if c.reads == 2 {
		close(c.waiting)
	}
	return c.Conn.Read(p)
}

// failingListener fails every Accept and signals Close on closed.
type failingListener struct {
	closed chan struct{}
}

func (l failingListener) Accept() (net.Conn, error) { return nil, errors.New("accept failed") }
func (l failingListener) Close() error              { close(l.closed); return nil }
func (l failingListener) Addr() net.Addr            { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1} }

func TestServeDrainsWritesBeforeStoppingPacing(t *testing.T) {
	ln := newBodyListener(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- testbench.Serve(ctx, ln, config("/tools/ais")) }()

	type result struct {
		status int
		body   string
	}
	response := make(chan result, 1)
	requestBody, sendBody := io.Pipe()
	go func() {
		// A failed call would block the test; report it through the result instead.
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, "http://"+ln.Addr().String()+"/tools/ais/api/vessels", requestBody)
		if err != nil {
			response <- result{body: err.Error()}
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			response <- result{body: err.Error()}
			return
		}
		data, err := io.ReadAll(resp.Body)
		if err = errors.Join(err, resp.Body.Close()); err != nil {
			data = append(data, err.Error()...)
		}
		response <- result{status: resp.StatusCode, body: string(data)}
	}()
	<-ln.waiting // The write handler waits for its body.

	cancel()
	<-ln.closed // Shutdown has started.
	_, err := io.WriteString(sendBody, `{"count":2}`)
	require.NoError(t, err)
	require.NoError(t, sendBody.Close())

	got := <-response
	require.Equal(t, http.StatusOK, got.status, "pacing still runs while requests drain: %s", got.body)
	require.NoError(t, <-done)
}

func TestServeClosesListenerOnFailure(t *testing.T) {
	ln := failingListener{closed: make(chan struct{})}
	err := testbench.Serve(t.Context(), ln, config("/tools/"))
	require.ErrorContains(t, err, "create test bench")
	<-ln.closed

	ln = failingListener{closed: make(chan struct{})}
	// Serve returns only after the server and the pacing loop have stopped.
	err = testbench.Serve(t.Context(), ln, config(""))
	require.ErrorContains(t, err, "accept failed")
	<-ln.closed
}

func TestServeStopsOnCancellation(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- testbench.Serve(ctx, ln, config("")) }()

	// The listener is already bound, so requests queue until serving starts.
	status, body := call(t, http.MethodGet, "http://"+ln.Addr().String()+"/display", "")
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "<h1>Display</h1>")
	cancel()
	require.NoError(t, <-done)
	_, err = net.Dial("tcp", ln.Addr().String())
	require.Error(t, err, "Serve closed its listener")
}

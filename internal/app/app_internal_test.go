package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// trackedListener wraps a loopback listener. It counts accepted connections,
// signals the first one on accepted, holds every connection's reads until
// release is closed, and signals Close on closed.
type trackedListener struct {
	net.Listener
	accepts   atomic.Int64
	accepted  chan struct{}
	release   chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
}

// newTrackedListener returns a listener whose connections read immediately
// when released is true, and only after release is closed otherwise.
func newTrackedListener(t *testing.T, released bool) *trackedListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	l := &trackedListener{Listener: ln, accepted: make(chan struct{}, 1), release: make(chan struct{}), closed: make(chan struct{})}
	if released {
		close(l.release)
	}
	return l
}

func (l *trackedListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.accepts.Add(1)
	select {
	case l.accepted <- struct{}{}:
	default:
	}
	return gatedConn{Conn: conn, release: l.release}, nil
}

func (l *trackedListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return l.Listener.Close()
}

// gatedConn blocks reads until release is closed.
type gatedConn struct {
	net.Conn
	release <-chan struct{}
}

func (c gatedConn) Read(p []byte) (int, error) {
	<-c.release
	return c.Conn.Read(p)
}

// failingListener fails every Accept and signals Close on closed.
type failingListener struct {
	closed chan struct{}
}

func (l failingListener) Accept() (net.Conn, error) { return nil, errors.New("accept failed") }
func (l failingListener) Close() error              { close(l.closed); return nil }
func (l failingListener) Addr() net.Addr            { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1} }

// get returns the status code and body of a GET request.
func get(t *testing.T, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode, string(body)
}

// startServe runs serve until the returned cancel is called; wait returns its
// result.
func startServe(t *testing.T, public, internal net.Listener) (cancel func(), wait func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, slog.New(slog.DiscardHandler), public, internal)
	}()
	return cancel, func() error { return <-done }
}

// put returns the status code and body of a JSON PUT request.
func put(t *testing.T, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode, string(data)
}

func TestSpeedControlsSharedEngine(t *testing.T) {
	public, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	internal := newTrackedListener(t, true)
	cancel, wait := startServe(t, public, internal)
	publicAPI, internalAPI := "http://"+public.Addr().String()+"/api", "http://"+internal.Addr().String()+"/api"

	status, body := put(t, internalAPI+"/time", `{"speed":2.5}`)
	require.Equal(t, http.StatusOK, status, body)
	status, body = get(t, publicAPI+"/metadata")
	require.Equal(t, http.StatusOK, status, body)
	require.Contains(t, body, `"speed":2.5,"paused":false`)

	status, body = put(t, publicAPI+"/time", `{"speed":0}`)
	require.Equal(t, http.StatusOK, status, body)
	status, body = get(t, internalAPI+"/metadata")
	require.Equal(t, http.StatusOK, status, body)
	require.Contains(t, body, `"speed":0,"paused":true`, "both listeners control one engine")

	// Shutdown uses real time while the simulation is paused.
	cancel()
	require.NoError(t, wait())
}

func TestDisplayReadsPrivateListener(t *testing.T) {
	public, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	internal := newTrackedListener(t, true)
	cancel, wait := startServe(t, public, internal)
	base := "http://" + public.Addr().String()

	status, _ := get(t, base+"/api/vessels")
	require.Equal(t, http.StatusOK, status)
	require.Zero(t, internal.accepts.Load(), "public API requests do not use the private listener")

	status, body := get(t, base+"/display/api/observations")
	require.Equal(t, http.StatusOK, status, body)
	require.Positive(t, internal.accepts.Load(), "display client reads the private listener")

	status, body = get(t, base+"/display/api/stations/unknown/receptions?simulationId=x")
	require.Equal(t, http.StatusConflict, status, body)

	status, _ = get(t, "http://"+internal.Addr().String()+"/manager")
	require.Equal(t, http.StatusNotFound, status, "private listener serves API routes only")

	cancel()
	require.NoError(t, wait())
}

func TestShutdownDrainsInFlightDisplayRequest(t *testing.T) {
	public := newTrackedListener(t, true)
	internal := newTrackedListener(t, false)
	cancel, wait := startServe(t, public, internal)

	type result struct {
		status int
		body   string
	}
	response := make(chan result, 1)
	go func() {
		status, body := get(t, "http://"+public.Addr().String()+"/display/api/observations")
		response <- result{status, body}
	}()
	<-internal.accepted // The display request waits on its simulator read.

	cancel()
	<-public.closed // Shutdown has started.
	close(internal.release)

	got := <-response
	require.Equal(t, http.StatusOK, got.status, got.body)
	require.Contains(t, got.body, `"stations":[{`)
	require.NoError(t, wait())
}

func TestServeStopsAllComponentsWhenServingFails(t *testing.T) {
	tests := []struct {
		name       string
		failPublic bool
		wantErr    string
	}{
		{name: "public", failPublic: true, wantErr: "public server"},
		{name: "internal", failPublic: false, wantErr: "internal simulator API server"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failing := failingListener{closed: make(chan struct{})}
			healthy := newTrackedListener(t, true)
			var public, internal net.Listener = healthy, failing
			if tt.failPublic {
				public, internal = failing, healthy
			}

			// serve returns only after both servers and the pacing loop have stopped.
			err := serve(t.Context(), slog.New(slog.DiscardHandler), public, internal)

			require.ErrorContains(t, err, tt.wantErr)
			require.ErrorContains(t, err, "accept failed")
			<-failing.closed
			<-healthy.closed
		})
	}
}

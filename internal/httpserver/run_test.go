package httpserver_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/httpserver"
)

// closeListener signals Close on closed.
type closeListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func newCloseListener(t *testing.T) *closeListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return &closeListener{Listener: ln, closed: make(chan struct{})}
}

func (l *closeListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return l.Listener.Close()
}

func TestRunDrainsRequestsBeforeStoppingWork(t *testing.T) {
	ln := newCloseListener(t)
	entered, release, workStopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		select {
		case <-workStopped:
			_, _ = io.WriteString(w, "stopped")
		default:
			_, _ = io.WriteString(w, "running")
		}
	})
	work := func(ctx context.Context) error {
		<-ctx.Done()
		close(workStopped)
		return nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- httpserver.Run(ctx, slog.New(slog.DiscardHandler), ln, handler, work) }()

	body := make(chan string, 1)
	go func() {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String(), nil)
		if err != nil {
			body <- err.Error()
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			body <- err.Error()
			return
		}
		data, err := io.ReadAll(resp.Body)
		body <- string(data) + errString(errors.Join(err, resp.Body.Close()))
	}()
	<-entered
	cancel()
	<-ln.closed // Shutdown has started.
	close(release)

	require.Equal(t, "running", <-body, "work keeps running while requests drain")
	require.NoError(t, <-done)
	<-workStopped
}

func TestRunStopsServingWhenWorkFails(t *testing.T) {
	ln := newCloseListener(t)

	err := httpserver.Run(t.Context(), slog.New(slog.DiscardHandler), ln, http.NotFoundHandler(), func(context.Context) error {
		return errors.New("pacing failed")
	})

	require.ErrorContains(t, err, "pacing failed")
	<-ln.closed
}

func TestRunStopsWorkWhenServingFails(t *testing.T) {
	ln := failingListener{closed: make(chan struct{})}
	workStopped := make(chan struct{})

	err := httpserver.Run(t.Context(), slog.New(slog.DiscardHandler), ln, http.NotFoundHandler(), func(ctx context.Context) error {
		<-ctx.Done()
		close(workStopped)
		return nil
	})

	require.ErrorContains(t, err, "accept failed")
	<-ln.closed
	select {
	case <-workStopped:
	default:
		t.Fatal("Run returned before work was joined")
	}
}

func TestRunWithoutWorkStopsOnCancellation(t *testing.T) {
	ln := newCloseListener(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.NoError(t, httpserver.Run(ctx, slog.New(slog.DiscardHandler), ln, http.NotFoundHandler(), nil))
	<-ln.closed
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return ": " + err.Error()
}

package httpserver_test

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/httpserver"
)

func TestServeUntilShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := httpserver.Serve(slog.New(slog.DiscardHandler), ln, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String(), nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "ok", string(body))

	require.NoError(t, server.Shutdown(t.Context()))
	<-server.Done()
	require.Error(t, ln.Close(), "the server closes its listener")
}

// failingListener fails every Accept and reports Close on closed.
type failingListener struct {
	closed chan struct{}
}

func (l failingListener) Accept() (net.Conn, error) { return nil, errors.New("accept failed") }
func (l failingListener) Close() error              { close(l.closed); return nil }
func (l failingListener) Addr() net.Addr            { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

func TestShutdownReportsServeFailure(t *testing.T) {
	ln := failingListener{closed: make(chan struct{})}
	server := httpserver.Serve(slog.New(slog.DiscardHandler), ln, http.NotFoundHandler())

	<-server.Done()
	err := server.Shutdown(t.Context())

	require.ErrorContains(t, err, "accept failed")
	<-ln.closed
}

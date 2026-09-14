package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const (
	readHeaderTimeout = 5 * time.Second
	// ShutdownTimeout is the graceful shutdown budget of one process, shared by
	// all of its servers.
	ShutdownTimeout = 5 * time.Second
)

// Server serves one listener in its own goroutine.
type Server struct {
	srv  *http.Server
	done chan struct{}
	err  error // Result of http.Server.Serve; read only after done closes.
}

// Serve starts serving handler on ln and returns immediately. The server owns
// ln and closes it when serving ends. Call Shutdown once to stop it and collect
// its result.
func Serve(logger *slog.Logger, ln net.Listener, handler http.Handler) *Server {
	s := &Server{
		srv: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
			ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
		},
		done: make(chan struct{}),
	}
	go func() {
		s.err = s.srv.Serve(ln)
		close(s.done)
	}()
	return s
}

// Done is closed when serving has ended, through Shutdown or a failure.
func (s *Server) Done() <-chan struct{} {
	return s.done
}

// Shutdown stops accepting connections and waits for in-flight requests until
// ctx ends, then closes the remaining connections. It waits for serving to end
// and also returns the serving failure when serving ended for another reason.
func (s *Server) Shutdown(ctx context.Context) error {
	var err error
	if shutdownErr := s.srv.Shutdown(ctx); shutdownErr != nil {
		err = errors.Join(fmt.Errorf("shutdown http: %w", shutdownErr), s.srv.Close())
	}
	<-s.done
	if !errors.Is(s.err, http.ErrServerClosed) {
		err = errors.Join(err, fmt.Errorf("serve http: %w", s.err))
	}
	return err
}

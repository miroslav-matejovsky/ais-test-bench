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

// Run serves handler on ln next to work until ctx is cancelled, serving fails,
// or work returns. It then drains in-flight requests within ShutdownTimeout of
// real time while work keeps running, so draining requests can still use it.
// Only then is work's context cancelled and work joined. Work's context keeps
// ctx's values but not its cancellation. A nil work runs nothing. Run owns ln
// and returns serving, shutdown, and work failures joined, or nil after a clean
// shutdown.
func Run(ctx context.Context, logger *slog.Logger, ln net.Listener, handler http.Handler, work func(context.Context) error) error {
	server := Serve(logger, ln, handler)
	workCtx, stopWork := context.WithCancel(context.WithoutCancel(ctx))
	defer stopWork()
	var workDone chan struct{} // Nil without work, so the select never picks it.
	var workErr error          // Read only after workDone closes.
	if work != nil {
		workDone = make(chan struct{})
		go func() {
			workErr = work(workCtx)
			close(workDone)
		}()
	}

	select {
	case <-server.Done():
	case <-workDone:
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ShutdownTimeout)
	defer cancel()
	err := server.Shutdown(shutdownCtx)
	stopWork()
	if workDone != nil {
		<-workDone
	}
	return errors.Join(err, workErr)
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

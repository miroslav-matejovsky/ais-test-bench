package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/ui"
)

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 5 * time.Second
)

// Run serves the UI on ln until ctx is cancelled, then waits up to
// shutdownTimeout for in-flight requests. Run takes ownership of ln and closes
// it. It returns nil after a clean shutdown.
func Run(ctx context.Context, logger *slog.Logger, ln net.Listener) error {
	handler, err := ui.NewHandler(logger)
	if err != nil {
		return errors.Join(fmt.Errorf("create ui handler: %w", err), ln.Close())
	}

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	serveErr := make(chan error, 1)
	go func() {
		// Serve closes ln when it returns.
		serveErr <- srv.Serve(ln)
	}()
	logger.Info("server started", "url", "http://"+ln.Addr().String())

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}

	logger.Info("server shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve http: %w", err)
	}
	logger.Info("server stopped")
	return nil
}

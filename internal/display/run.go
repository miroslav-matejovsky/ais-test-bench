package display

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/httpserver"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/ui"
)

// NewHandler returns the standalone display routes backed by client:
//
//	GET /              redirect to /display
//	GET /display       display page
//	GET /static/       embedded static files
//	    /display/api/  display API, see NewAPI
func NewHandler(logger *slog.Logger, client *Client) (http.Handler, error) {
	pages, err := ui.NewPages(logger, []ui.Link{{Href: "/display", Label: "Display"}})
	if err != nil {
		return nil, fmt.Errorf("create pages: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/display/api/", NewAPI(logger, client))
	mux.Handle("GET /static/", ui.Static())
	mux.Handle("GET /{$}", http.RedirectHandler("/display", http.StatusFound))
	mux.HandleFunc("GET /display", pages.Display)
	return mux, nil
}

// Run serves the standalone display (see NewHandler) on ln until ctx is
// cancelled or serving fails. Run takes ownership of ln and closes it, also
// when construction fails. Shutdown waits up to httpserver.ShutdownTimeout for
// in-flight requests, which cancels their simulator reads when it expires, and
// then closes idle simulator connections. An unreachable simulator does not
// stop Run; each request reports it. Run returns nil after a clean shutdown.
func Run(ctx context.Context, logger *slog.Logger, ln net.Listener, client *Client) error {
	handler, err := NewHandler(logger, client)
	if err != nil {
		return errors.Join(fmt.Errorf("create display handler: %w", err), ln.Close())
	}
	defer client.CloseIdleConnections()

	server := httpserver.Serve(logger, ln, handler)
	logger.Info("display started", "url", "http://"+ln.Addr().String(), "simulator", client.Origin())
	select {
	case <-server.Done():
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), httpserver.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("display stopped")
	return nil
}

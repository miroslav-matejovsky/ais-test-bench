package simulator

import (
	"fmt"
	"net/http"

	"github.com/miroslav-matejovsky/ais-testbench/internal/ui"
)

// NewHandler returns the standalone simulator routes for sim, logging to sim's
// logger:
//
//	GET /           redirect to /manager
//	GET /manager    manager page
//	GET /status     server status, a fragment for htmx partial requests
//	GET /static/    embedded static files
//	    /api/       simulator API, see Simulator.API
func NewHandler(sim *Simulator) (http.Handler, error) {
	pages, err := ui.NewPages(sim.logger, []ui.Link{{Href: "/manager", Label: "Manager"}})
	if err != nil {
		return nil, fmt.Errorf("create pages: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", sim.API())
	mux.Handle("GET /static/", ui.Static())
	mux.Handle("GET /{$}", http.RedirectHandler("/manager", http.StatusFound))
	mux.HandleFunc("GET /manager", pages.Manager)
	mux.HandleFunc("GET /status", pages.Status)
	return mux, nil
}

package simulator

import (
	"fmt"
	"net/http"

	"github.com/miroslav-matejovsky/ais-testbench/internal/urlpath"
	"github.com/miroslav-matejovsky/ais-testbench/ui"
)

// NewStandaloneHandler returns the standalone simulator routes for sim below
// basePath, logging to sim's logger. basePath is empty for the root or a public
// prefix such as "/tools/ais", without a trailing "/":
//
//	GET {base}/           redirect to {base}/manager, keeping the query
//	GET {base}/manager    manager page
//	GET {base}/status     server status, a fragment for htmx partial requests
//	GET {base}/assets/    embedded assets
//	    {base}/api/       simulator API, see Simulator.API
//
// The handler matches full request paths; mount it without http.StripPrefix.
func NewStandaloneHandler(sim *Simulator, basePath string) (http.Handler, error) {
	if err := urlpath.CheckPrefix(basePath); err != nil {
		return nil, fmt.Errorf("base path: %w", err)
	}
	base := basePath
	pages, err := ui.New(ui.Config{
		ManagerAPIBase: base + "/api/", AssetsBase: base + "/assets/",
		HomeURL: base + "/", ManagerURL: base + "/manager", StatusURL: base + "/status", Logger: sim.baseLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("create pages: %w", err)
	}
	manager, err := pages.ManagerPage()
	if err != nil {
		return nil, fmt.Errorf("create manager page: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle(base+"/api/", http.StripPrefix(base+"/api", sim.API()))
	mux.Handle(base+"/assets/", http.StripPrefix(base+"/assets", pages.Assets()))
	mux.HandleFunc("GET "+base+"/{$}", func(w http.ResponseWriter, r *http.Request) {
		target := base + "/manager"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusFound)
	})
	mux.Handle(base+"/manager", manager)
	mux.Handle(base+"/status", pages.StatusPage())
	return mux, nil
}

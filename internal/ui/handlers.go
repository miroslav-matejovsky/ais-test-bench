package ui

import (
	"html/template"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/miroslav-matejovsky/ais-testbench/internal/ui/assets"
)

// Link is one entry of the page header navigation.
type Link struct {
	Href  string
	Label string
}

// Pages renders the HTML pages. It holds rendering state only: page scripts
// fetch their data from JSON APIs. Each component mounts the pages it serves
// and supplies local page links and any explicit external navigation.
type Pages struct {
	logger  *slog.Logger
	html    *renderer
	started time.Time
}

// NewPages parses the shared layout with nav as the header links of every page.
// Uptime reported by Status is measured from the NewPages call.
func NewPages(logger *slog.Logger, nav []Link) (*Pages, error) {
	links := slices.Clone(nav)
	funcs := template.FuncMap{"nav": func() []Link { return links }}
	html, err := newRenderer(assets.HTMLFiles, funcs, "base.tmpl")
	if err != nil {
		return nil, err
	}
	return &Pages{logger: logger, html: html, started: time.Now()}, nil
}

// Static serves the embedded static files. Mount it at /static/.
func Static() http.Handler {
	return http.StripPrefix("/static", http.FileServerFS(assets.StaticFiles))
}

// Home renders the entry page linking /manager and /display.
func (p *Pages) Home(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, nil, "base", "pages/home.tmpl")
}

// Manager renders the vessel count form and recent message viewer. Its script
// uses /api/vessels, /api/messages, and /api/metadata on the same origin.
func (p *Pages) Manager(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, nil, "base", "pages/manager.tmpl")
}

// Display renders the live map of received targets. Its script polls
// /display/api/observations on the same origin.
func (p *Pages) Display(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, nil, "base", "pages/display.tmpl")
}

// statusData is the view model for pages/status.tmpl.
type statusData struct {
	Now    time.Time
	Uptime time.Duration
}

// Status renders server time and uptime: a full page, or a fragment for htmx
// partial requests.
func (p *Pages) Status(w http.ResponseWriter, r *http.Request) {
	data := statusData{
		Now:    time.Now(),
		Uptime: time.Since(p.started).Round(time.Second),
	}
	name := "base"
	if isPartialRequest(r) {
		name = "status:details"
	}
	p.render(w, r, data, name, "pages/status.tmpl")
}

func (p *Pages) render(w http.ResponseWriter, r *http.Request, data any, name string, pageFiles ...string) {
	if err := p.html.render(w, http.StatusOK, data, name, pageFiles...); err != nil {
		p.logger.Error("render", "path", r.URL.Path, "template", name, "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// isPartialRequest reports whether htmx asked for a fragment to swap into a
// target element. History restores and body swaps send "full" instead.
func isPartialRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request-Type") == "partial"
}

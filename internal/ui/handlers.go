package ui

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/ui/assets"
)

type server struct {
	logger  *slog.Logger
	html    *renderer
	started time.Time
}

// NewHandler returns the UI routes:
//
//	GET /           home page linking both UIs
//	GET /manager    Manager UI
//	GET /display    Display UI
//	GET /status     server status, a fragment for htmx partial requests
//	GET /static/    embedded static files
//
// Uptime reported by /status is measured from the NewHandler call.
func NewHandler(logger *slog.Logger) (http.Handler, error) {
	html, err := newRenderer(assets.HTMLFiles, "base.tmpl")
	if err != nil {
		return nil, err
	}
	s := &server{logger: logger, html: html, started: time.Now()}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static", http.FileServerFS(assets.StaticFiles)))
	mux.HandleFunc("GET /{$}", s.page("pages/home.tmpl"))
	mux.HandleFunc("GET /manager", s.page("pages/manager.tmpl"))
	mux.HandleFunc("GET /display", s.page("pages/display.tmpl"))
	mux.HandleFunc("GET /status", s.status)
	return mux, nil
}

// page returns a handler rendering pageFile as a full page without data.
func (s *server) page(pageFile string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, r, nil, "base", pageFile)
	}
}

// statusData is the view model for pages/status.tmpl.
type statusData struct {
	Now    time.Time
	Uptime time.Duration
}

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	data := statusData{
		Now:    time.Now(),
		Uptime: time.Since(s.started).Round(time.Second),
	}
	name := "base"
	if isPartialRequest(r) {
		name = "status:details"
	}
	s.render(w, r, data, name, "pages/status.tmpl")
}

func (s *server) render(w http.ResponseWriter, r *http.Request, data any, name string, pageFiles ...string) {
	if err := s.html.render(w, http.StatusOK, data, name, pageFiles...); err != nil {
		s.logger.Error("render", "path", r.URL.Path, "template", name, "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// isPartialRequest reports whether htmx asked for a fragment to swap into a
// target element. History restores and body swaps send "full" instead.
func isPartialRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request-Type") == "partial"
}

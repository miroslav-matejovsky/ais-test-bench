package ui

import (
	"bytes"
	"cmp"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/miroslav-matejovsky/ais-testbench/ui/internal/assets"
)

// Leaflet is loaded from a CDN with subresource integrity until it is bundled.
const (
	leafletCSS          = "https://unpkg.com/leaflet@1.9.4/dist/leaflet.css"
	leafletCSSIntegrity = "sha256-p4NxAoJBhIIN+hmNHrzRCf9tD/miZyoHS5obTRR9BMY="
	leafletJS           = "https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"
	leafletJSIntegrity  = "sha256-20nQCchB9co0qIjJZRGuk2/Z9VM+kNiyxNV1lvTlZBo="
)

// Component root IDs used by the standalone pages.
const (
	managerPageID = "ais-manager"
	displayPageID = "ais-display"
)

// contentTypes fixes asset media types independent of the operating system's
// MIME registry.
var contentTypes = map[string]string{
	".css": "text/css; charset=utf-8",
	".js":  "text/javascript; charset=utf-8",
}

// UI renders manager and display components and standalone pages from embedded
// templates, and serves embedded assets. It holds no simulation state and is
// safe for concurrent use. Construct it with New.
type UI struct {
	config  Config
	logger  *slog.Logger
	shared  *template.Template            // Document and component templates.
	pages   map[string]*template.Template // Page name to its template set.
	started time.Time                     // Status uptime origin.
}

// componentData is the view model of a component template.
type componentData struct {
	ID         string
	APIBase    string
	ManagerURL string
}

// pageData is the view model of the standalone document.
type pageData struct {
	HomeURL    string
	ManagerURL string
	DisplayURL string
	StatusURL  string
	Resources  Resources
	Component  template.HTML // Output of RenderManager or RenderDisplay.
	Status     statusData
}

// statusData is the status page view model.
type statusData struct {
	Now    time.Time
	Uptime time.Duration
}

// New validates config and parses the embedded templates. It binds no routes,
// starts no goroutines, and changes no process globals. Status uptime is
// measured from New.
func New(config Config) (*UI, error) {
	if err := validate(config); err != nil {
		return nil, fmt.Errorf("invalid UI configuration: %w", err)
	}
	shared, err := template.ParseFS(assets.HTMLFiles, "document.tmpl", "components/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse shared templates: %w", err)
	}
	pages := make(map[string]*template.Template)
	for _, name := range []string{"home", "manager", "display", "status"} {
		set, err := shared.Clone()
		if err != nil {
			return nil, fmt.Errorf("clone shared templates: %w", err)
		}
		if _, err := set.ParseFS(assets.HTMLFiles, "pages/"+name+".tmpl"); err != nil {
			return nil, fmt.Errorf("parse %s page: %w", name, err)
		}
		pages[name] = set
	}
	return &UI{
		config: config, logger: cmp.Or(config.Logger, slog.Default()).With("component", "ui"),
		shared: shared, pages: pages, started: time.Now(),
	}, nil
}

// Assets serves embedded stylesheets and scripts at local paths such as
// /css/app.css. Mount it at AssetsBase and strip that base once:
//
//	mux.Handle("/tools/ais/assets/", http.StripPrefix("/tools/ais/assets", u.Assets()))
//
// It answers GET and HEAD, returns 405 otherwise, and 404 for directories and
// unknown files. CSS and JavaScript have fixed content types.
func (u *UI) Assets() http.Handler {
	files := http.FileServerFS(assets.StaticFiles)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !readMethod(w, r) {
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		info, err := fs.Stat(assets.StaticFiles, name)
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		if contentType, ok := contentTypes[path.Ext(name)]; ok {
			w.Header().Set("Content-Type", contentType)
		}
		files.ServeHTTP(w, r)
	})
}

// ManagerResources lists the stylesheet and scripts a host page loads for a
// manager component.
func (u *UI) ManagerResources() Resources {
	return Resources{
		Stylesheets: []Resource{{URL: u.config.AssetsBase + "css/app.css"}},
		Scripts:     []Resource{{URL: u.config.AssetsBase + "js/manager.js"}, {URL: u.config.AssetsBase + "js/stations.js"}},
	}
}

// DisplayResources lists the stylesheets and scripts a host page loads for a
// display component, including Leaflet from its CDN with integrity values. The
// display keeps its tables working when Leaflet cannot load.
func (u *UI) DisplayResources() Resources {
	return Resources{
		Stylesheets: []Resource{{URL: leafletCSS, Integrity: leafletCSSIntegrity}, {URL: u.config.AssetsBase + "css/app.css"}},
		Scripts:     []Resource{{URL: leafletJS, Integrity: leafletJSIntegrity}, {URL: u.config.AssetsBase + "js/display.js"}},
	}
}

// RenderManager writes the manager component markup for a host template: one
// root element with the given ID and the escaped API base as an inert data
// attribute, without a document shell or scripts. It fails without writing when
// ManagerAPIBase is not configured or the ID is invalid.
func (u *UI) RenderManager(w io.Writer, config ComponentConfig) error {
	data, err := u.component("manager", config, u.config.ManagerAPIBase)
	if err != nil {
		return err
	}
	return execute(w, u.shared, "manager", data)
}

// RenderDisplay writes the display component markup, as RenderManager does,
// using DisplayAPIBase and the optional ManagerURL link.
func (u *UI) RenderDisplay(w io.Writer, config ComponentConfig) error {
	data, err := u.component("display", config, u.config.DisplayAPIBase)
	if err != nil {
		return err
	}
	return execute(w, u.shared, "display", data)
}

// HomePage returns the standalone home page handler, linking the configured
// manager and display pages.
func (u *UI) HomePage() http.Handler {
	data := u.pageData()
	return u.page("home", "document", func(*http.Request) pageData { return data })
}

// ManagerPage returns the standalone manager page handler. The page embeds the
// RenderManager output, so standalone and host pages share one component. It
// fails when ManagerAPIBase is not configured.
func (u *UI) ManagerPage() (http.Handler, error) {
	var component bytes.Buffer
	if err := u.RenderManager(&component, ComponentConfig{ID: managerPageID}); err != nil {
		return nil, err
	}
	data := u.pageData()
	data.Component = template.HTML(component.String()) // Escaped by the component template.
	data.Resources = u.ManagerResources()
	if u.config.StatusURL != "" {
		data.Resources.Scripts = append(data.Resources.Scripts, Resource{URL: u.config.AssetsBase + "js/htmx.min.js"})
	}
	return u.page("manager", "document", func(*http.Request) pageData { return data }), nil
}

// DisplayPage returns the standalone display page handler, embedding the
// RenderDisplay output. It fails when DisplayAPIBase is not configured.
func (u *UI) DisplayPage() (http.Handler, error) {
	var component bytes.Buffer
	if err := u.RenderDisplay(&component, ComponentConfig{ID: displayPageID}); err != nil {
		return nil, err
	}
	data := u.pageData()
	data.Component = template.HTML(component.String()) // Escaped by the component template.
	data.Resources = u.DisplayResources()
	return u.page("display", "document", func(*http.Request) pageData { return data }), nil
}

// StatusPage returns the status handler: server time and uptime as a full page,
// or as a fragment when htmx sends HX-Request-Type: partial. History restores
// send "full" and get the page. Responses vary by HX-Request-Type.
func (u *UI) StatusPage() http.Handler {
	base := u.pageData()
	handler := func(fragment bool) http.Handler {
		name := "document"
		if fragment {
			name = "status:details"
		}
		return u.page("status", name, func(*http.Request) pageData {
			data := base
			data.Status = statusData{Now: time.Now(), Uptime: time.Since(u.started).Round(time.Second)}
			return data
		})
	}
	full, partial := handler(false), handler(true)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("HX-Request-Type") == "partial" {
			partial.ServeHTTP(w, r)
			return
		}
		full.ServeHTTP(w, r)
	})
}

// component validates a component request against its configured API base.
func (u *UI) component(name string, config ComponentConfig, apiBase string) (componentData, error) {
	if apiBase == "" {
		return componentData{}, fmt.Errorf("render %s: API base is not configured", name)
	}
	if !componentID.MatchString(config.ID) {
		return componentData{}, fmt.Errorf("render %s: invalid component ID %q", name, config.ID)
	}
	return componentData{ID: config.ID, APIBase: apiBase, ManagerURL: u.config.ManagerURL}, nil
}

// pageData returns the links and default resources shared by every page.
func (u *UI) pageData() pageData {
	return pageData{
		HomeURL: u.config.HomeURL, ManagerURL: u.config.ManagerURL, DisplayURL: u.config.DisplayURL,
		StatusURL: u.config.StatusURL,
		Resources: Resources{Stylesheets: []Resource{{URL: u.config.AssetsBase + "css/app.css"}}},
	}
}

// page returns a GET and HEAD handler rendering template name from a page set.
// Rendering is buffered: a failure is logged and returns 500 without partial HTML.
func (u *UI) page(page, name string, data func(*http.Request) pageData) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !readMethod(w, r) {
			return
		}
		u.render(w, r, page, name, data(r))
	})
}

func (u *UI) render(w http.ResponseWriter, r *http.Request, page, name string, data pageData) {
	var buf bytes.Buffer
	if err := u.pages[page].ExecuteTemplate(&buf, name, data); err != nil {
		u.logger.ErrorContext(r.Context(), "render", "path", r.URL.Path, "template", name, "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Add("Vary", "HX-Request-Type")
	if _, err := buf.WriteTo(w); err != nil {
		u.logger.DebugContext(r.Context(), "write page", "path", r.URL.Path, "error", err)
	}
}

// execute renders into a buffer so a failure writes nothing to w.
func execute(w io.Writer, set *template.Template, name string, data componentData) error {
	var buf bytes.Buffer
	if err := set.ExecuteTemplate(&buf, name, data); err != nil {
		return fmt.Errorf("render %s: %w", name, err)
	}
	if _, err := buf.WriteTo(w); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

// readMethod allows GET and HEAD and writes 405 with Allow otherwise.
func readMethod(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	return false
}

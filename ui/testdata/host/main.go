// Command host serves a hostile host page for the optional browser check
// host-browser.mjs. It embeds two independent simulators below /a/ and /b/, each
// with a manager and a display component, through the public simulator, display,
// and ui packages. The page carries old component IDs and unrelated forms,
// tables, and navigation, loads no inline scripts, and is served with a strict
// Content-Security-Policy. Host middleware rejects every API request without the
// instance's X-Host-CSRF header, which only the page's custom fetch adds.
//
//	go run ./ui/testdata/host -addr 127.0.0.1:18090
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/miroslav-matejovsky/ais-testbench/display"
	"github.com/miroslav-matejovsky/ais-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/ais-testbench/simulator"
	"github.com/miroslav-matejovsky/ais-testbench/ui"
)

const assetsBase = "/shared/assets/"

// csp allows only same-origin scripts and styles, and OpenStreetMap tiles.
const csp = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: https://tile.openstreetmap.org; " +
	"connect-src 'self'; base-uri 'none'; form-action 'self'"

var page = template.Must(template.New("host").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Host operations</title>
<link rel="stylesheet" href="{{.Stylesheet}}">
<script type="module" src="/host.js"></script>
</head>
<body data-ais-module="{{.Module}}">
<nav id="nav"><a href="/">Host home</a> <a href="/reports">Reports</a></nav>
<h1 id="heading">Host operations</h1>
<form id="vessel-form" action="/" method="get">
<label for="vessel-count">Host vessel filter</label>
<input id="vessel-count" name="count" value="host">
<button id="apply-count" type="submit">Host search</button>
</form>
<table id="stations-table"><caption>Host table</caption><tbody id="stations-body"><tr><td>Host row</td></tr></tbody></table>
<div id="map">Host map placeholder</div>
<pre id="messages">Host messages</pre>
<div id="slot-fleet-a" data-host-token="token-a">{{.FleetA}}</div>
<div id="slot-fleet-b" data-host-token="token-b">{{.FleetB}}</div>
<div id="slot-map-a" data-host-token="token-a">{{.MapA}}</div>
<div id="slot-map-b" data-host-token="token-b" hidden>{{.MapB}}</div>
</body>
</html>
`))

// hostScript mounts every component with a per-root fetch that adds the host
// token and records requests. hold delays requests of the listed roots without
// honoring their abort signal, so the check can destroy during a request.
const hostScript = `const ui = await import(document.body.dataset.aisModule);
const host = window.aisHost = { ui, requests: [], handles: {}, holdIds: new Set(), held: [] };
host.release = () => host.held.splice(0).forEach(entry => entry.resolve());
host.mount = id => {
    const root = document.getElementById(id);
    const token = root.closest("[data-host-token]").dataset.hostToken;
    const fetch = async (url, init) => {
        host.requests.push({ id, url, method: init.method || "GET" });
        if (host.holdIds.has(id)) await new Promise(resolve => host.held.push({ id, resolve }));
        const headers = new Headers(init.headers);
        headers.set("X-Host-CSRF", token);
        return window.fetch(url, { ...init, headers });
    };
    const mount = root.hasAttribute("data-ais-manager") ? ui.mountManager : ui.mountDisplay;
    return host.handles[id] = mount(root, { fetch });
};
for (const id of ["fleet-a", "fleet-b", "map-a", "map-b"]) host.mount(id);
host.ready = true;
`

func main() {
	addr := flag.String("addr", "127.0.0.1:18090", "listen address")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, *addr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// instance is one embedded bench below its own prefix.
type instance struct {
	sim *simulator.Simulator
	ui  *ui.UI
}

func run(ctx context.Context, addr string) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	mux := http.NewServeMux()
	instances := make(map[string]instance)
	for _, name := range []string{"a", "b"} {
		inst, err := mountInstance(mux, logger, name)
		if err != nil {
			return err
		}
		instances[name] = inst
	}
	mux.Handle(assetsBase, http.StripPrefix(assetsBase[:len(assetsBase)-1], instances["a"].ui.Assets()))
	mux.HandleFunc("GET /host.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write([]byte(hostScript))
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		body, err := render(instances["a"].ui, instances["b"].ui)
		if err != nil {
			logger.Error("render host page", "error", err)
			http.Error(w, "render failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(body)
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, len(instances))
	for _, inst := range instances {
		go func() { errs <- inst.sim.Run(runCtx) }()
	}
	go func() {
		<-runCtx.Done()
		shutdownCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer done()
		_ = server.Shutdown(shutdownCtx)
	}()
	logger.Info("host started", "url", "http://"+ln.Addr().String())
	err = server.Serve(ln)
	cancel()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	for range instances {
		err = errors.Join(err, <-errs)
	}
	return err
}

// mountInstance mounts one simulator API and display API below /{name}/,
// protected by the host token middleware.
func mountInstance(mux *http.ServeMux, logger *slog.Logger, name string) (instance, error) {
	sim, err := simulator.New(simulator.Config{Simulation: simdriver.NewConfig(), Logger: logger})
	if err != nil {
		return instance{}, err
	}
	client, err := display.New(sim)
	if err != nil {
		return instance{}, err
	}
	displayAPI, err := display.NewHandler(display.Config{Client: client, Logger: logger})
	if err != nil {
		return instance{}, err
	}
	prefix := "/" + name
	u, err := ui.New(ui.Config{
		ManagerAPIBase: prefix + "/api/", DisplayAPIBase: prefix + "/display/api/", AssetsBase: assetsBase, Logger: logger,
	})
	if err != nil {
		return instance{}, err
	}
	token := requireToken("token-" + name)
	mux.Handle(prefix+"/api/", token(http.StripPrefix(prefix+"/api", sim.API())))
	mux.Handle(prefix+"/display/api/", token(http.StripPrefix(prefix+"/display/api", displayAPI)))
	return instance{sim: sim, ui: u}, nil
}

// requireToken is host CSRF middleware.
func requireToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Host-CSRF") != token {
				http.Error(w, "missing host token", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func render(a, b *ui.UI) ([]byte, error) {
	var fleetA, fleetB, mapA, mapB bytes.Buffer
	if err := errors.Join(
		a.RenderManager(&fleetA, ui.ComponentConfig{ID: "fleet-a"}),
		b.RenderManager(&fleetB, ui.ComponentConfig{ID: "fleet-b"}),
		a.RenderDisplay(&mapA, ui.ComponentConfig{ID: "map-a"}),
		b.RenderDisplay(&mapB, ui.ComponentConfig{ID: "map-b"}),
	); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	err := page.Execute(&out, map[string]any{
		"Stylesheet": a.StylesheetURL(), "Module": a.ModuleURL(),
		"FleetA": template.HTML(fleetA.String()), "FleetB": template.HTML(fleetB.String()), // #nosec G203 -- Rendered by ui.
		"MapA": template.HTML(mapA.String()), "MapB": template.HTML(mapB.String()), // #nosec G203 -- Rendered by ui.
	})
	return out.Bytes(), err
}

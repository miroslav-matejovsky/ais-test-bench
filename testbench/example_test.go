package testbench_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/miroslav-matejovsky/ais-testbench/display"
	"github.com/miroslav-matejovsky/ais-testbench/simulator"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
	"github.com/miroslav-matejovsky/ais-testbench/testbench"
	"github.com/miroslav-matejovsky/ais-testbench/ui"
)

// Example mounts a bench below /tools/ais on a host mux next to the host's own
// routes, behind host authentication middleware. The host owns its server and
// supervises the bench's pacing; see supervise for the complete lifecycle.
func Example() {
	if err := hostMux(); err != nil {
		fmt.Println(err)
	}
	// Output:
	// 401 Unauthorized
	// 200 OK
}

func hostMux() error {
	bench, err := testbench.New(testbench.Config{
		Simulation: simulator.DemoConfig(),
		Logger:     slog.New(slog.DiscardHandler),
		BasePath:   "/tools/ais",
	})
	if err != nil {
		return fmt.Errorf("create bench: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "host home")
	})
	// Handler matches full request paths: mount it without http.StripPrefix.
	mux.Handle("/tools/ais/", requireOperator(bench.Handler()))

	return serveHost(mux, []func(context.Context) error{bench.Run}, func(base string) error {
		for _, token := range []string{"", "operator"} {
			status, _, err := send(http.MethodGet, base+"/tools/ais/manager", token, "")
			if err != nil {
				return err
			}
			fmt.Println(status)
		}
		return nil
	})
}

// operationsPage is a host template. Components are trusted HTML rendered by ui;
// the host's own module script, /static/operations.js, imports data-ais-module
// and mounts both roots (see package ui).
var operationsPage = template.Must(template.New("operations").Parse(`<!doctype html>
<title>Operations</title>
<link rel="stylesheet" href="{{.Stylesheet}}">
<script type="module" src="/static/operations.js"></script>
<main data-ais-module="{{.Module}}">
<section>{{.Fleet}}</section>
<section>{{.Traffic}}</section>
</main>
`))

// Example_hostTemplate renders the bench's manager and display components into a
// host page. Bench.UI already knows the bench's API and asset URLs.
func Example_hostTemplate() {
	if err := hostTemplate(); err != nil {
		fmt.Println(err)
	}
	// Output:
	// 200 OK
	// true
	// true
	// true
}

func hostTemplate() error {
	bench, err := testbench.New(testbench.Config{
		Simulation: simulator.DemoConfig(),
		Logger:     slog.New(slog.DiscardHandler),
		BasePath:   "/tools/ais",
	})
	if err != nil {
		return fmt.Errorf("create bench: %w", err)
	}
	u := bench.UI()
	mux := http.NewServeMux()
	mux.Handle("/tools/ais/", bench.Handler())
	mux.HandleFunc("GET /operations", func(w http.ResponseWriter, _ *http.Request) {
		// Render into buffers so a failure can still change the response status.
		var fleet, traffic, page bytes.Buffer
		if err := errors.Join(
			u.RenderManager(&fleet, ui.ComponentConfig{ID: "fleet"}),
			u.RenderDisplay(&traffic, ui.ComponentConfig{ID: "traffic"}),
		); err != nil {
			http.Error(w, "render failed", http.StatusInternalServerError)
			return
		}
		err := operationsPage.Execute(&page, map[string]any{
			"Stylesheet": u.StylesheetURL(), "Module": u.ModuleURL(),
			"Fleet": template.HTML(fleet.String()), "Traffic": template.HTML(traffic.String()), // #nosec G203 -- ui escapes rendered components.
		})
		if err != nil {
			http.Error(w, "render failed", http.StatusInternalServerError)
			return
		}
		_, _ = page.WriteTo(w) // A failed write means the client went away.
	})

	return serveHost(mux, []func(context.Context) error{bench.Run}, func(base string) error {
		status, body, err := send(http.MethodGet, base+"/operations", "", "")
		if err != nil {
			return err
		}
		fmt.Println(status)
		fmt.Println(strings.Contains(body, `<div id="fleet" class="ais-manager" data-ais-manager data-api-base="/tools/ais/api/">`))
		fmt.Println(strings.Contains(body, `<div id="traffic" class="ais-display" data-ais-display data-api-base="/tools/ais/display/api/"`))
		fmt.Println(strings.Contains(body, `<main data-ais-module="/tools/ais/assets/js/ui.js">`))
		return nil
	})
}

// Example_remoteDisplay serves a display in one host that reads a simulator
// mounted below /tools/ais in another host. The simulator host requires a service
// token, which the display host's own HTTP client adds; browser credentials are
// never forwarded.
func Example_remoteDisplay() {
	if err := remoteDisplay(); err != nil {
		fmt.Println(err)
	}
	// Output: 200 OK 3
}

func remoteDisplay() error {
	logger := slog.New(slog.DiscardHandler)
	bench, err := testbench.New(testbench.Config{Simulation: simulator.DemoConfig(), Logger: logger, BasePath: "/tools/ais"})
	if err != nil {
		return fmt.Errorf("create bench: %w", err)
	}
	simulatorHost := http.NewServeMux()
	simulatorHost.Handle("/tools/ais/", requireOperator(bench.Handler()))

	return serveHost(simulatorHost, []func(context.Context) error{bench.Run}, func(simulatorURL string) error {
		defaults, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return errors.New("default transport is not an *http.Transport")
		}
		// The display host owns this transport and closes it after its server
		// stops; display never closes a supplied client.
		transport := defaults.Clone()
		defer transport.CloseIdleConnections()
		source, err := display.NewHTTPSource(display.HTTPConfig{
			APIBase: simulatorURL + "/tools/ais/api/",
			Client:  &http.Client{Transport: bearer{token: "operator", next: transport}},
		})
		if err != nil {
			return fmt.Errorf("create source: %w", err)
		}
		client, err := display.New(source)
		if err != nil {
			return fmt.Errorf("create display client: %w", err)
		}
		api, err := display.NewHandler(display.Config{Client: client, Logger: logger})
		if err != nil {
			return fmt.Errorf("create display API: %w", err)
		}
		pages, err := ui.New(ui.Config{
			DisplayAPIBase: "/traffic/api/", AssetsBase: "/traffic/assets/",
			ManagerURL: simulatorURL + "/tools/ais/manager", Logger: logger,
		})
		if err != nil {
			return fmt.Errorf("create pages: %w", err)
		}
		page, err := pages.DisplayPage()
		if err != nil {
			return fmt.Errorf("create display page: %w", err)
		}
		displayHost := http.NewServeMux()
		displayHost.Handle("/traffic/api/", http.StripPrefix("/traffic/api", api))
		displayHost.Handle("/traffic/assets/", http.StripPrefix("/traffic/assets", pages.Assets()))
		displayHost.Handle("/traffic", page)

		return serveHost(displayHost, nil, func(displayURL string) error {
			status, body, err := send(http.MethodGet, displayURL+"/traffic/api/observations?stations=all", "", "")
			if err != nil {
				return err
			}
			var observations display.Observations
			if err := json.Unmarshal([]byte(body), &observations); err != nil {
				return fmt.Errorf("decode observations: %s: %w", status, err)
			}
			fmt.Println(status, len(observations.Stations))
			return nil
		})
	})
}

// Example_independentBenches serves two benches with separate engines below
// their own prefixes on one server. A change to one does not reach the other.
func Example_independentBenches() {
	if err := independentBenches(); err != nil {
		fmt.Println(err)
	}
	// Output:
	// harbour 3
	// offshore 1
}

func independentBenches() error {
	names := []string{"harbour", "offshore"}
	mux := http.NewServeMux()
	var runs []func(context.Context) error
	for _, name := range names {
		bench, err := testbench.New(testbench.Config{
			Simulation: simulator.DemoConfig(),
			Logger:     slog.New(slog.DiscardHandler).With("bench", name),
			BasePath:   "/" + name,
		})
		if err != nil {
			return fmt.Errorf("create %s bench: %w", name, err)
		}
		mux.Handle("/"+name+"/", bench.Handler())
		runs = append(runs, bench.Run)
	}

	return serveHost(mux, runs, func(base string) error {
		status, body, err := send(http.MethodPut, base+"/harbour/api/vessels", "", `{"count":3}`)
		if err != nil {
			return err
		}
		if status != "200 OK" {
			return fmt.Errorf("set harbour vessel count: %s: %s", status, body)
		}
		for _, name := range names {
			_, body, err := send(http.MethodGet, base+"/"+name+"/api/vessels", "", "")
			if err != nil {
				return err
			}
			var fleet simulatorapi.Fleet
			if err := json.Unmarshal([]byte(body), &fleet); err != nil {
				return fmt.Errorf("decode %s fleet: %w", name, err)
			}
			fmt.Println(name, len(fleet.Vessels))
		}
		return nil
	})
}

// supervise is a host's serving lifecycle. It serves handler on ln and runs every
// bench's pacing until ctx ends, serving fails, or pacing fails. It then drains
// requests while pacing still runs, cancels and joins pacing, and returns every
// failure. The server closes ln.
func supervise(ctx context.Context, ln net.Listener, handler http.Handler, runs ...func(context.Context) error) error {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	served := make(chan error, 1)
	go func() { served <- server.Serve(ln) }()
	// Pacing ignores ctx cancellation, so draining writes still reach the engine.
	paceCtx, stopPacing := context.WithCancel(context.WithoutCancel(ctx))
	defer stopPacing()
	paced := make(chan error, len(runs))
	for _, run := range runs {
		go func() { paced <- run(paceCtx) }()
	}

	var errs []error
	pending := len(runs)
	select {
	case <-ctx.Done():
	case err := <-served:
		served <- err // Collected with the shutdown result below.
	case err := <-paced:
		errs = append(errs, err) // Run returns nil only after cancellation.
		pending--
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	errs = append(errs, server.Shutdown(shutdownCtx))
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		errs = append(errs, err)
	}
	stopPacing()
	for range pending {
		errs = append(errs, <-paced)
	}
	return errors.Join(errs...)
}

// serveHost supervises handler and runs on a loopback listener while visit uses
// the running host, then shuts down. A real host waits for SIGINT or SIGTERM
// instead of visit.
func serveHost(handler http.Handler, runs []func(context.Context) error, visit func(base string) error) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- supervise(ctx, ln, handler, runs...) }()
	err = visit("http://" + ln.Addr().String())
	stop()
	return errors.Join(err, <-done)
}

// requireOperator is host authentication middleware. A real host checks its
// session and also requires a CSRF token for writes.
func requireOperator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer operator" {
			http.Error(w, "login required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// bearer adds service credentials to every request of a host-owned client.
type bearer struct {
	token string
	next  http.RoundTripper
}

func (b bearer) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(req)
}

// send makes one request with an optional bearer token and JSON body and returns
// the response status and body.
func send(method, url, token, body string) (string, string, error) {
	req, err := http.NewRequestWithContext(context.Background(), method, url, strings.NewReader(body))
	if err != nil {
		return "", "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	data, err := io.ReadAll(resp.Body)
	return resp.Status, string(data), errors.Join(err, resp.Body.Close())
}

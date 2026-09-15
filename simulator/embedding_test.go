package simulator_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/display"
	"github.com/miroslav-matejovsky/ais-testbench/simulation"
	"github.com/miroslav-matejovsky/ais-testbench/simulator"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
	"github.com/miroslav-matejovsky/ais-testbench/ui"
)

// layout is the public and mounted address of every embedded handler. Public
// values are browser URLs; mount values are the host mux patterns below which
// each handler strips its prefix. They differ only behind a stripping proxy.
type layout struct {
	managerPage, displayPage       string
	managerAPI, displayAPI, assets string // Public base paths ending in "/".
	mountPrefix                    string // Removed by the proxy before the host mux.
}

func (l layout) mount(public string) string { return strings.TrimPrefix(public, l.mountPrefix) }

func newBench(t *testing.T, id string) *simulator.Simulator {
	t.Helper()
	sim, err := simulator.New(simulator.Config{Logger: slog.New(slog.DiscardHandler), Simulation: simulation.Config{
		ID: id, StartTime: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), Seed: 7, InitialVesselCount: 1,
		Transmitter: simulation.TransmitterProfile{PowerWatts: 12.5, HeightMeters: 10, GainDBi: 2, FeederLossDB: 1},
		Stations: []simulation.StationDefinition{{
			Name: "Coast", Latitude: 52.01, Longitude: 3.97, Enabled: true, AntennaHeightMeters: 25, ReceiveGainDBi: 3, FeederLossDB: 2,
			ChannelA: simulation.ReceiverChannel{Enabled: true, SensitivityDBm: -110}, ChannelB: simulation.ReceiverChannel{Enabled: true, SensitivityDBm: -110},
		}},
	}})
	require.NoError(t, err)
	return sim
}

// embed mounts one bench on mux with public API calls only. The display reads
// the simulator in process, so no HTTP server is involved.
func embed(t *testing.T, mux *http.ServeMux, sim *simulator.Simulator, l layout) {
	t.Helper()
	client, err := display.New(sim)
	require.NoError(t, err)
	displayAPI, err := display.NewHandler(display.Config{Client: client})
	require.NoError(t, err)
	pages, err := ui.New(ui.Config{
		ManagerAPIBase: l.managerAPI, DisplayAPIBase: l.displayAPI, AssetsBase: l.assets,
		ManagerURL: l.managerPage, DisplayURL: l.displayPage, Logger: slog.New(slog.DiscardHandler),
	})
	require.NoError(t, err)
	managerPage, err := pages.ManagerPage()
	require.NoError(t, err)
	displayPage, err := pages.DisplayPage()
	require.NoError(t, err)
	for public, handler := range map[string]http.Handler{l.managerAPI: sim.API(), l.displayAPI: displayAPI, l.assets: pages.Assets()} {
		mounted := l.mount(public)
		mux.Handle(mounted, http.StripPrefix(strings.TrimSuffix(mounted, "/"), handler))
	}
	mux.Handle(l.mount(l.managerPage), managerPage)
	mux.Handle(l.mount(l.displayPage), displayPage)
}

func nested(prefix string) layout {
	return layout{
		managerPage: prefix + "/manager", displayPage: prefix + "/display",
		managerAPI: prefix + "/api/", displayAPI: prefix + "/display/api/", assets: prefix + "/assets/",
	}
}

func do(handler http.Handler, method, target, body string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range header {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// hostMux returns a host mux that owns / and /api/ itself.
func hostMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "host root") })
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "host api") })
	return mux
}

// checkBench verifies pages, assets, both APIs, a relative station Location, and
// query preservation through one public layout served by handler.
func checkBench(t *testing.T, handler http.Handler, l layout, runID string) {
	t.Helper()
	manager := do(handler, http.MethodGet, l.managerPage, "", nil)
	require.Equal(t, http.StatusOK, manager.Code, manager.Body.String())
	require.Contains(t, manager.Body.String(), `data-api-base="`+l.managerAPI+`"`)
	require.Contains(t, manager.Body.String(), `<script type="module" src="`+l.assets+`js/standalone.js"></script>`)
	require.Contains(t, manager.Body.String(), `<link rel="stylesheet" href="`+l.assets+`css/ui.css">`)
	require.Contains(t, manager.Body.String(), `href="`+l.managerAPI+`messages"`)
	page := do(handler, http.MethodGet, l.displayPage, "", nil)
	require.Equal(t, http.StatusOK, page.Code)
	require.Contains(t, page.Body.String(), `data-api-base="`+l.displayAPI+`"`)
	require.Contains(t, page.Body.String(), `href="`+l.managerPage+`">Open manager</a>`)

	asset := do(handler, http.MethodGet, l.assets+"js/display.js", "", nil)
	require.Equal(t, http.StatusOK, asset.Code)
	require.Equal(t, "text/javascript; charset=utf-8", asset.Header().Get("Content-Type"))

	var metadata simulatorapi.Metadata
	rec := do(handler, http.MethodGet, l.managerAPI+"metadata", "", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &metadata))
	require.Equal(t, runID, metadata.SimulationID)

	selected := do(handler, http.MethodGet, l.displayAPI+"observations?stations=station-1", "", nil)
	require.Equal(t, http.StatusOK, selected.Code, selected.Body.String())
	var observations display.Observations
	require.NoError(t, json.Unmarshal(selected.Body.Bytes(), &observations))
	require.Equal(t, []string{"station-1"}, observations.Selection, "the query survives prefix stripping")
	require.Equal(t, runID, observations.SimulationID)

	body := `{"simulationId":"` + runID + `","stationSetRevision":"1","definition":{"name":"Harbour","latitude":51.95,"longitude":4.14,"enabled":true,` +
		`"antennaHeightMeters":15,"receiveGainDbi":2,"feederLossDb":3,"channelA":{"enabled":true,"sensitivityDbm":-108,"noisePenaltyDb":0,"dropProbability":0},` +
		`"channelB":{"enabled":true,"sensitivityDbm":-108,"noisePenaltyDb":0,"dropProbability":0},"shadowSectors":[]}}`
	created := do(handler, http.MethodPost, l.managerAPI+"stations", body, nil)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	requestURL, err := url.Parse("https://host.example" + l.managerAPI + "stations")
	require.NoError(t, err)
	location, err := url.Parse(created.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, l.managerAPI+"stations/station-2", requestURL.ResolveReference(location).Path)
	require.Equal(t, http.StatusOK, do(handler, http.MethodGet, requestURL.ResolveReference(location).Path+"/receptions?simulationId="+runID, "", nil).Code)
}

func TestEmbeddedLayouts(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		mux := http.NewServeMux()
		l := nested("")
		embed(t, mux, newBench(t, "root"), l)
		checkBench(t, mux, l, "root")
	})
	t.Run("nested prefix with host routes", func(t *testing.T) {
		mux := hostMux()
		l := nested("/tools/ais")
		embed(t, mux, newBench(t, "nested"), l)
		checkBench(t, mux, l, "nested")
		require.Equal(t, "host root", do(mux, http.MethodGet, "/", "", nil).Body.String())
		require.Equal(t, "host api", do(mux, http.MethodGet, "/api/metadata", "", nil).Body.String())
		require.Equal(t, "host root", do(mux, http.MethodGet, "/manager", "", nil).Body.String())
		redirect := do(mux, http.MethodGet, "/tools/ais/api?view=1", "", nil)
		require.GreaterOrEqual(t, redirect.Code, http.StatusMovedPermanently)
		require.Less(t, redirect.Code, http.StatusBadRequest)
		require.Equal(t, "/tools/ais/api/?view=1", redirect.Header().Get("Location"), "the host mux keeps prefix and query")
		require.Equal(t, http.StatusNotFound, do(mux, http.MethodGet, "/tools/ais/api/", "", nil).Code)
		require.Equal(t, http.StatusMethodNotAllowed, do(mux, http.MethodDelete, "/tools/ais/api/vessels", "", nil).Code)
	})
	t.Run("independent instances", func(t *testing.T) {
		mux := hostMux()
		first, second := nested("/first"), nested("/second")
		embed(t, mux, newBench(t, "first-run"), first)
		embed(t, mux, newBench(t, "second-run"), second)
		checkBench(t, mux, first, "first-run")
		checkBench(t, mux, second, "second-run")
		require.Equal(t, http.StatusOK, do(mux, http.MethodPut, "/first/api/vessels", `{"count":3}`, nil).Code)
		var fleet simulatorapi.Fleet
		require.NoError(t, json.Unmarshal(do(mux, http.MethodGet, "/second/api/vessels", "", nil).Body.Bytes(), &fleet))
		require.Len(t, fleet.Vessels, 1, "writes to one instance do not reach the other")
	})
	t.Run("separately mounted UI, APIs, and assets", func(t *testing.T) {
		mux := hostMux()
		l := layout{
			managerPage: "/ops/fleet", displayPage: "/ops/map",
			managerAPI: "/backend/simulator/v1/", displayAPI: "/backend/received/", assets: "/static-ais/",
		}
		embed(t, mux, newBench(t, "separate"), l)
		checkBench(t, mux, l, "separate")
	})
	t.Run("proxy strips external prefix", func(t *testing.T) {
		backend := http.NewServeMux()
		l := nested("/external/ais")
		l.mountPrefix = "/external/ais"
		embed(t, backend, newBench(t, "proxied"), l)
		proxy := http.NewServeMux()
		proxy.Handle("/external/ais/", http.StripPrefix("/external/ais", backend))
		checkBench(t, proxy, l, "proxied")
		require.Contains(t, do(backend, http.MethodGet, "/manager", "", nil).Body.String(), `data-api-base="/external/ais/api/"`,
			"public URLs come from configuration, not from the request path")
	})
}

func TestHostMiddlewareProtectsEmbeddedHandlers(t *testing.T) {
	sim := newBench(t, "protected")
	client, err := display.New(sim)
	require.NoError(t, err)
	displayAPI, err := display.NewHandler(display.Config{Client: client})
	require.NoError(t, err)
	// Host policy: every request needs a session; writes also need a CSRF token.
	protect := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer session" {
				http.Error(w, "login required", http.StatusUnauthorized)
				return
			}
			if r.Method != http.MethodGet && r.Header.Get("X-CSRF-Token") != "token" {
				http.Error(w, "csrf token required", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	mux := http.NewServeMux()
	mux.Handle("/ais/api/", protect(http.StripPrefix("/ais/api", sim.API())))
	mux.Handle("/ais/display/api/", protect(http.StripPrefix("/ais/display/api", displayAPI)))

	session := map[string]string{"Authorization": "Bearer session"}
	write := map[string]string{"Authorization": "Bearer session", "X-CSRF-Token": "token"}
	for _, tt := range []struct {
		method, target, body string
		header               map[string]string
		wantStatus           int
	}{
		{http.MethodGet, "/ais/api/vessels", "", nil, http.StatusUnauthorized},
		{http.MethodGet, "/ais/display/api/observations", "", nil, http.StatusUnauthorized},
		{http.MethodPut, "/ais/api/vessels", `{"count":2}`, session, http.StatusForbidden},
		{http.MethodPut, "/ais/api/time", `{"speed":2}`, session, http.StatusForbidden},
		{http.MethodGet, "/ais/api/vessels", "", session, http.StatusOK},
		{http.MethodGet, "/ais/display/api/observations", "", session, http.StatusOK},
		{http.MethodPut, "/ais/api/vessels", `{"count":2}`, write, http.StatusOK},
	} {
		rec := do(mux, tt.method, tt.target, tt.body, tt.header)
		require.Equal(t, tt.wantStatus, rec.Code, "%s %s: %s", tt.method, tt.target, rec.Body.String())
		for key := range rec.Header() {
			require.False(t, strings.HasPrefix(key, "Access-Control-"), "library handlers set no CORS policy: %s", key)
		}
		require.Empty(t, rec.Header().Get("Www-Authenticate"))
	}
	require.Len(t, sim.Fleet().Vessels, 2, "only the authorized write changed the engine")
}

// authTransport adds host credentials and records the path and deadline of
// every upstream request.
type authTransport struct {
	mu        sync.Mutex
	paths     []string
	deadlines []bool
}

func (a *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	_, hasDeadline := req.Context().Deadline()
	a.mu.Lock()
	a.paths = append(a.paths, req.URL.Path)
	a.deadlines = append(a.deadlines, hasDeadline)
	a.mu.Unlock()
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer service")
	return http.DefaultTransport.RoundTrip(req)
}

func TestRemoteSourceReadsPrefixedAuthenticatedSimulator(t *testing.T) {
	sim := newBench(t, "remote")
	upstream := http.NewServeMux()
	upstream.Handle("/tools/ais/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer service" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.StripPrefix("/tools/ais/api", sim.API()).ServeHTTP(w, r)
	}))
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)

	transport := &authTransport{}
	httpClient := &http.Client{Transport: transport}
	source, err := display.NewHTTPSource(display.HTTPConfig{APIBase: server.URL + "/tools/ais/api/", Client: httpClient})
	require.NoError(t, err)
	client, err := display.New(source)
	require.NoError(t, err)
	remote, err := client.Observations(t.Context(), []string{"station-1"})
	require.NoError(t, err)
	local, err := display.New(sim)
	require.NoError(t, err)
	want, err := local.Observations(t.Context(), []string{"station-1"})
	require.NoError(t, err)
	require.Equal(t, want, remote)
	_, err = client.ReceptionHistory(t.Context(), "station-1", simulatorapi.HistoryRequest{SimulationID: "remote"})
	require.NoError(t, err)
	require.Equal(t, []string{"/tools/ais/api/observations", "/tools/ais/api/stations/station-1/receptions"}, transport.paths)
	require.Equal(t, []bool{true, true}, transport.deadlines, "every upstream read has a deadline")

	source.CloseIdleConnections()
	require.Same(t, transport, httpClient.Transport, "a borrowed client is not replaced")

	unauthenticated, err := display.NewHTTPSource(display.HTTPConfig{APIBase: server.URL + "/tools/ais/api/"})
	require.NoError(t, err)
	t.Cleanup(unauthenticated.CloseIdleConnections)
	_, err = unauthenticated.Observations(t.Context(), nil)
	require.ErrorIs(t, err, simulatorapi.ErrInvalidResponse, "credentials come only from the configured HTTP client")
	missingPrefix, err := display.NewHTTPSource(display.HTTPConfig{APIBase: server.URL + "/api/", Client: httpClient})
	require.NoError(t, err)
	_, err = missingPrefix.Observations(t.Context(), nil)
	require.ErrorIs(t, err, simulatorapi.ErrInvalidResponse)
}

# Embeddable UI and public runtime

## Goal and value

Let Go applications embed the manager and display in their existing HTTP server
and page layout. Consumers supply their simulation configuration, logger, route
prefixes, middleware, and lifecycle. The commands use these same public APIs.

Status: steps 01-04 implemented and verified. Steps 05-06 remain planned.
Breaking changes are allowed. See each step's status for results and the current API.

## Constraints at the start of the refactor

- `simulation` is the only public library package. Its `Config` has no logger.
- `internal/simulator` combines API handlers, the manager, and server lifecycle.
  Its handlers require the inaccessible `internal/simdriver.Driver`.
- `internal/display` combines the HTTP client, validation, projection, routes,
  and server lifecycle. Its client accepts only an origin, excluding mounted APIs.
- `internal/ui` embeds templates and assets but assumes `/static`, `/api`, and
  `/display/api`. Templates include a complete document and application navigation.
- Browser scripts use document-wide IDs, start themselves, and retain polling
  timers. Styles target `body`, forms, tables, and other host-page elements.
- Most internal handlers and runners already accept `*slog.Logger`. Commands
  create the logger. This existing plumbing should be retained and made public.
- Combined mode opens a private loopback listener for display reads. Shutdown
  ordering, driver settlement, and display validation have existing regression tests.

## Scope and decisions

Support both complete pages mounted under a prefix and components rendered inside
a consumer's page. An iframe is optional, not the embedding API. Support manager
only, display only, both together, and multiple independent instances per server
and page. Keep embedded assets and the existing frontend without a Node build.

### Package boundaries

All paths below are relative to `github.com/miroslav-matejovsky/ais-testbench`.

| Package | Public responsibility |
| --- | --- |
| `simulation` | Existing deterministic engine; optional logger configuration |
| `simulator` | Engine ownership, serialized real-time driver, simulator API, local observation source, explicit `Run(ctx)` |
| `simulatorapi` | Shared wire DTOs, history query and typed source errors needed by public source contracts |
| `display` | Observation source interface, remote HTTP source, shared validation and decoding, display API |
| `ui` | Embedded assets, page and component rendering, URL configuration, navigation and browser mounting contract |
| `testbench` | Small composition facade for common setups and standalone serving |

Keep codecs, rendering helpers, driver implementation, and HTTP lifecycle helpers
internal where they do not appear in public signatures. Move existing implementations;
avoid maintaining parallel public and internal versions. Define the two-method
observation source interface in `display`, its consumer. Its signatures use public
`simulatorapi` request/response types and `context.Context`. The simulator can
satisfy it structurally without importing `display`.

### Proposed consumer API

The following signatures define the intended shape; implementation must document
every option and lifecycle rule:

```go
// Package testbench.
type Config struct {
    Simulation simulation.Config
    Logger     *slog.Logger
    BasePath   string // Public absolute path, e.g. "/tools/ais"; empty means root.
}

func New(Config) (*Bench, error)
func (b *Bench) Handler() http.Handler
func (b *Bench) Run(context.Context) error // Pacing only; caller runs its server.
func Serve(context.Context, net.Listener, Config) error // Standalone convenience.
```

Example mounting inside an existing server, with error handling included:

```go
func attachAIS(mux *http.ServeMux, cfg simulation.Config, logger *slog.Logger) (*testbench.Bench, error) {
    bench, err := testbench.New(testbench.Config{
        Simulation: cfg,
        Logger: logger,
        BasePath: "/tools/ais",
    })
    if err != nil {
        return nil, err
    }
    mux.Handle("/tools/ais/", bench.Handler())
    return bench, nil
}
```

The host supervises `bench.Run(ctx)` alongside its server, handles its returned
error, drains requests, then cancels and joins pacing. The example's handler
expects the original path; do not also use `http.StripPrefix`. The host may wrap
that handler in authentication, authorization, request logging, and CSRF middleware.

Lower-level APIs expose `simulator.New(Config)`, `Simulator.API()`,
`Simulator.Run(ctx)`, `display.NewHTTPSource(HTTPConfig)`,
`display.NewHandler(Config)`, and `ui.New(Config)`. The facade uses those same
entry points. Low-level API handlers use local route paths; the facade strips
its configured prefix once. Document the exact mounting recipe for each level.

`ui` provides `Assets() http.Handler`, full-page HTTP handlers, and
`RenderManager(io.Writer, ComponentConfig) error` /
`RenderDisplay(io.Writer, ComponentConfig) error` for host templates. Components
contain only their own root and markup. Their config includes a unique instance
ID and API base URL. Render to a buffer before sending a host response if rendering
errors must still change its HTTP status. Hosts load the published stylesheet and
module once and explicitly mount each rendered root:

```js
import { mountManager, mountDisplay } from "/tools/ais/assets/ui.js";

const manager = mountManager(document.getElementById("ais-manager"));
const display = mountDisplay(document.getElementById("ais-display"));
// On host navigation or component removal:
manager.destroy();
display.destroy();
```

Publish module URL and stylesheet URL through the Go UI object so consumers need
not assemble asset paths. Mount options support a supplied fetch function for
host authentication and CSRF headers; default to same-origin browser fetch.
Standalone pages use this same component API and lifecycle.

### URLs and assets

- Combined defaults: `{base}/manager`, `{base}/display`, `{base}/api/*`,
  `{base}/display/api/*`, and `{base}/assets/*`. Home/status are facade pages.
- Explicit `ManagerAPIBase`, `DisplayAPIBase`, and `AssetsBase` support separately
  mounted UI and APIs. Navigation is optional configuration, never inferred from
  a backend-only address. All template links and browser requests use these values.
- Validate configuration before serving. Base paths are clean absolute URL paths;
  reject dot segments, encoded separators, queries, fragments, and ambiguous
  trailing slashes. Specify one canonical trailing-slash rule for API/asset bases.
- Public paths are explicit, including a reverse proxy's external prefix. Do not
  infer them from forwarded headers. Cover a proxy that strips that prefix.
- Remote sources accept an HTTP(S) API base including its path, for example
  `https://example.test/tools/ais/api/`. Append relative endpoint paths without
  dropping the prefix. Reject user info, query, and fragment.
- Remote sources accept a caller-owned `*http.Client` for custom transport,
  authentication, and TLS. Preserve request deadlines and response size limits.
  Never mutate or close caller-owned transports. Internally created transports
  have explicit cleanup. No browser credentials are forwarded upstream implicitly.
- Scope CSS below a namespaced component root, use namespaced classes and unique
  label/control IDs, and expose a small documented set of CSS variables and sizing
  hooks. Application layout rules belong only to standalone page styles.
- Bundle a pinned Leaflet distribution with attribution/license in first-party
  assets outside `vendor`, isolating its module use from a host's `window.L`.
  Make tile URL and attribution configurable; preserve tables if maps fail.
  External scripts, inline executable configuration, and global htmx initialization
  must not be required for embedded components. Document tile/CSP requirements.

### Local and remote display reads

Use one public source contract returning received-observation wire snapshots and
reception history. The local simulator adapter converts committed snapshots using
existing wire conversion code. The remote source decodes bounded HTTP responses.
Both feed the same semantic validation and AIS decoding before display responses.

This intentionally replaces combined mode's mandatory loopback HTTP hop. Local
embedding starts no private listener. The display still receives only station
observations and NMEA, never the truth fleet. HTTP framing/body validation remains
specific to the remote source. Source errors must distinguish invalid requests,
missing stations, stale runs, unavailable sources, and invalid responses so existing
400/404/409/503/502 behavior survives. Cancellation propagates through both paths.

### Lifecycle and logging

- Constructors validate and allocate only. They do not bind sockets, start pacing
  or polling goroutines, install signal handlers, or change process globals.
- A simulator owns one engine; driver operations remain its only mutators.
  Accept `simulation.Config`, avoiding uncontrolled engine sharing. Consumers of
  the standalone engine keep the existing manual pacing model.
- `Run(ctx)` is explicit, single-use, and reports failures. Reject a second run.
  Define writes after termination to return an unavailable error while snapshots
  remain readable. Clean cancellation returns nil. No stored contexts.
- Standalone serving helpers explicitly take listener ownership, drain requests,
  stop and join pacing, and close owned resources within a bounded shutdown budget.
  Construction and partial-start failures release only owned resources.
- Add `Logger *slog.Logger` to behavior-owning public configs, including
  `simulation.Config`. Nil resolves to `slog.Default()` at construction. Never call
  `slog.SetDefault`, create a stderr logger, or install a handler in library code.
- A facade logger applies to all children, including the engine; document that it
  overrides a nested engine logger. Independently constructed packages use their
  own config. Preserve caller attributes, groups, levels, and context values.
- Use contextual slog methods where a context exists. Derive component loggers
  with a consistent `component` field. Returned operational errors are logged by
  the supervising caller; HTTP handlers log actionable failures they consume.
  Do not add per-tick, per-message, per-poll, or duplicate error logging.
- The engine remains deterministic in state and emitted bytes. Logger choice must
  not affect results. Do not invoke consumer handlers while holding engine/driver
  locks. Logger configuration alone does not require new engine log events.

## Implementation order

1. [Public runtime and source contracts](01-public-runtime.md)
2. [Logger injection](02-logging.md)
3. [Prefix-safe handlers and rendering](03-http-embedding.md)
4. [Host-page browser components](04-browser-components.md)
5. [Facade, commands, and consumer examples](05-composition.md)
6. [End-to-end verification and documentation](06-verification.md)

See [assessment](assessment.md) for impact and risks. No compatibility aliases,
framework adapters, transport publishing, or simulation-model changes are planned.

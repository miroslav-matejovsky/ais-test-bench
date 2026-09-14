# Bootstrap and lifecycle

This is the implementation blueprint. Current Go files define data and interfaces;
the current executable only reports scaffold status. No sockets are opened yet.

## Initialization order

1. Load `config.yaml`, apply defaults, reject unknown keys, validate configuration.
2. Initialize `slog` using the configured format and level.
3. Open scenario and vessel repositories; register reverse-order cleanup.
4. Construct movement models, simulator, AIS generator/encoder, management, and
   visualization services. Resolve autostart references before starting workers.
5. Initialize the TCP publisher, bind its listener if enabled, set connection limits.
6. Initialize the UDP publisher, resolve destinations and configure its socket.
7. Construct REST handlers and middleware; build the `/api/v1` router.
8. Load the embedded admin asset subtree and mount `/admin` if enabled.
9. Load the embedded viewer asset subtree and mount `/viewer` if enabled.
10. Bind and start HTTP; supervise HTTP, TCP, UDP, and simulation workers in one
    `errgroup.WithContext`. Start an autostart scenario only after transports are
    ready. Mark healthy after initialization and worker readiness.
11. On process cancellation or fatal worker error, perform graceful shutdown and
    return the original failure plus any cleanup errors.

Services may be constructed before publishers, but the AIS publication coordinator
is finalized after both publisher dependencies exist. No worker runs with missing
dependencies. Bootstrap failures close everything already acquired in reverse order.

## Example main.go

This entry-point template uses the generated contracts. The two wiring variables
are intentionally unset until concrete adapters exist; it fails explicitly in that
state. Replace them with concrete constructors during implementation. The current
entry point in cmd is a smaller scaffold-status command.

```go
package main

import (
    "context"
    "flag"
    "fmt"
    "log/slog"
    "os"
    "os/signal"
    "syscall"

    "github.com/miroslav-matejovsky/ais-test-bench/internal/app"
)

// Concrete constructor calls replace these declarations in the implementation phase.
var loader app.ConfigLoader
var bootstrapper app.Bootstrapper

func main() {
    if err := run(); err != nil {
        slog.Error("ais-test-bench failed", "error", err)
        os.Exit(1)
    }
}

func run() error {
    path := flag.String("config", "config.yaml", "configuration file")
    flag.Parse()

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    if loader == nil || bootstrapper == nil {
        return fmt.Errorf("bootstrap adapters have not been wired")
    }
    cfg, err := loader.Load(ctx, *path)
    if err != nil {
        return fmt.Errorf("load configuration: %w", err)
    }

    var level slog.Level
    if err := level.UnmarshalText([]byte(cfg.Logging.Level)); err != nil {
        return fmt.Errorf("logging level: %w", err)
    }
    options := &slog.HandlerOptions{Level: level}
    var handler slog.Handler
    switch cfg.Logging.Format {
    case "json":
        handler = slog.NewJSONHandler(os.Stderr, options)
    case "text":
        handler = slog.NewTextHandler(os.Stderr, options)
    default:
        return fmt.Errorf("unsupported logging format %q", cfg.Logging.Format)
    }
    slog.SetDefault(slog.New(handler))

    runtime, err := bootstrapper.Build(ctx, cfg)
    if err != nil {
        return fmt.Errorf("bootstrap: %w", err)
    }
    // Runtime.Run supervises workers and completes shutdown before returning.
    return runtime.Run(ctx)
}
```

## Example router setup

This construction example is kept in documentation to leave the scaffold limited
to interfaces. The API subrouter uses relative paths such as `GET /scenarios`.
Use `GET /{$}` for its exact API root to avoid catching unknown paths.
Validate required handlers before calling this function.

```go
package api

import "net/http"

func newRouter(routes Routes) http.Handler {
    mux := http.NewServeMux()
    api := http.StripPrefix("/api/v1", routes.API)
    mux.Handle("/api/v1", http.RedirectHandler("/api/v1/", http.StatusPermanentRedirect))
    mux.Handle("/api/v1/", api)
    if routes.Admin != nil {
        mux.Handle("GET /admin", http.RedirectHandler("/admin/", http.StatusPermanentRedirect))
        mux.Handle("GET /admin/", http.StripPrefix("/admin/", routes.Admin))
    }
    if routes.Viewer != nil {
        mux.Handle("GET /viewer", http.RedirectHandler("/viewer/", http.StatusPermanentRedirect))
        mux.Handle("GET /viewer/", http.StripPrefix("/viewer/", routes.Viewer))
    }
    mux.Handle("GET /healthz", routes.Health)
    mux.Handle("GET /metrics", routes.Metrics)
    return mux
}
```

Construct each static handler with `fs.Sub(admin.Files, "assets")` or
`fs.Sub(viewer.Files, "assets")`, check its error and the required `index.html`,
then pass the subtree to `http.FileServerFS`.
The frontend asset directories remain separate. Never serve the repository root.
Use a private Prometheus registry and HTTP handler for metrics.

| Route | Owner and intended behavior |
| --- | --- |
| /admin | Redirect to /admin/ and serve the management frontend |
| /viewer | Redirect to /viewer/ and serve the visualization frontend |
| /api/v1/scenarios | Management scenario CRUD |
| /api/v1/targets | Management vessel CRUD |
| /api/v1/simulation | Status and explicit start/pause/resume/stop/seek/speed commands |
| /api/v1/viewer/targets | Consistent projected snapshots and target selection queries |
| /api/v1/viewer/charts | Available local chart metadata |
| /api/v1/system | Effective configuration and system status |
| /healthz | 200 after startup while essential workers are healthy; 503 during drain/failure |
| /metrics | Prometheus exposition for process, simulation, queues, and connections |

Wrap handlers with request IDs, panic recovery, access logs, finite body limits,
and metrics. Request contexts reach use cases and repositories. The JSON API
maps typed application errors, not raw storage or socket errors. Loopback defaults
keep the local management surface local; deliberate external exposure requires a
separate deployment access policy.

## Concurrency and shutdown contract

Runtime.Run creates the errgroup and owns its cancellation. Workers receive contexts
as arguments, never as struct fields. Use explicit worker readiness signals, a
single simulation state owner, and bounded queues. Fatal worker errors cancel
siblings; individual TCP disconnects remain local connection events.

Runtime.Run triggers Shutdown on a signal or worker failure and waits for cleanup
before returning. Shutdown must also work when Run exits early. Internally, make
cleanup execute once. Do not put the shutdown coordinator in the worker group it
must join.

Shutdown marks the process unready, stops HTTP admission, stops simulation
production, then drains accepted requests and publisher queues. Cancel pacing and
close listeners to unblock I/O. Use a fresh `context.WithTimeout` based on
`context.WithoutCancel(ctx)` for the configured shutdown budget. Separate producer
cancellation from the transport drain budget, so group cancellation does not
discard queues immediately.

At the deadline force-close connections, abandon remaining queued output with an
observable count, and join workers. Close repositories last. Use `errors.Join`
to preserve both the worker error and cleanup errors. A normal requested stop
returns nil only if cleanup succeeds.

TCP has a bounded queue per client and disconnects slow clients; one client must
not stall virtual time. UDP uses bounded queues and reports local failures; receipt
by the remote endpoint cannot be guaranteed. No unbounded goroutine per message.
All transport ordering is within a publisher; TCP and UDP delivery is not atomic.

Tests control a fake pacing clock and explicit virtual time. Random movement uses
a fixed seed. Seek reconstructs state without emitting historical AIS traffic.

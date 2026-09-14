# Package catalog

All names below are relative to the module root. Interfaces use idiomatic Go
initialisms: `AISEncoder`, `TCPPublisher`, and `UDPPublisher`.
Every Go package has a `doc.go`. Directory-only context roots have a README.

| Package | Purpose and responsibilities | Public contracts/data | Allowed dependencies |
| --- | --- | --- | --- |
| cmd/ais-test-bench | One executable entry point and process exit status | main, wiring example | app, standard library |
| internal/app | Composition, rollback, resource ownership, supervision | ConfigLoader, Bootstrapper, Worker, Runtime, HTTPServer | All contexts, api, config, web, standard library, errgroup |
| internal/app/config | Immutable YAML schema and validation requirements | Config, Server, Simulation, TCP, UDP, Logging, UI | Standard library |
| internal/api | Mount REST, UIs, health, metrics | Routes | net/http, handlers, middleware |
| internal/api/handlers | Decode requests, invoke use cases, map errors/results | Concrete handlers planned | dto, management/service, visualization/service, simulation/application, net/http |
| internal/api/middleware | Request IDs, recovery, bounds, access logs, metrics | Middleware | net/http, logging, metrics libraries |
| internal/api/dto | Versioned JSON transport values | Error | Standard library |
| internal/simulation/domain | Scenarios, lifecycle state, playback snapshots, events | Scenario, State, Status, Event | Standard library |
| internal/simulation/application | Simulation commands, virtual time, scenario persistence, orchestration | Simulator, ScenarioRepository, Clock, TargetStepper, TrafficGenerator, EventBus | Own domain, targets/domain, standard library |
| internal/simulation/infrastructure | Scenario storage, pacing, event delivery, cross-context orchestration adapters | Concrete adapters planned | Own application/domain, targets/application/domain, ais/application/domain, storage libraries |
| internal/ais/domain | Semantic reports and complete NMEA output lines | Report, Sentence | Standard library |
| internal/ais/application | Due-message generation, encoding, outbound publication | Generator, AISEncoder, TCPPublisher, UDPPublisher | Own domain, standard library |
| internal/ais/infrastructure | Codec integration, NMEA framing and validation | Concrete adapters planned | Own application/domain, selected codec library |
| internal/targets/domain | Vessel identity, navigation, tracks, waypoints | Vessel, NavigationState, Track, Waypoint | Standard library |
| internal/targets/application | Saved vessel access and movement evaluation | VesselRepository, MovementModel | Own domain, standard library |
| internal/targets/infrastructure | Vessel persistence and movement adapters | Concrete adapters planned | Own application/domain, storage libraries |
| internal/networking/tcp | Listener, bounded client queues, stream writes, connection management | Concrete TCP adapter planned; implements AIS TCPPublisher | ais/domain, net, context, observability |
| internal/networking/udp | Datagram destinations, broadcast options, queues, deadlines | Concrete UDP adapter planned; implements AIS UDPPublisher | ais/domain, net, context, observability |
| internal/management/service | Validated CRUD, startup config views, system status | ScenarioManager, TargetManager, SystemReader, SystemStatus | simulation/application/domain, targets/application/domain, app/config |
| internal/visualization/service | Consistent target projections, selection queries, chart metadata | TargetReader, Viewer, ChartCatalog, Snapshot, TargetView, Target, Chart | targets/domain, standard library |
| web/admin | Independent embedded management distribution | Files (embed.FS) | embed |
| web/viewer | Independent embedded visualization distribution | Files (embed.FS) | embed |

"Planned" means a documented adapter slot, not a no-op implementation.
Configuration is a leaf schema package; management may read it without importing
the app composition package. Contexts never import HTTP handlers or frontend code.

## Boundary contracts

Repositories preserve contextual errors, return detached values, and make each
save atomic. Application services distinguish invalid input, missing entities,
and conflicts; HTTP maps these into 400, 404, and 409. Unexpected failures use 500
with an opaque request ID. Concrete error types will be added with behavior.

Management orchestrates saved definitions through repositories. Simulation owns
the active scenario and freezes initial vessel data. CRUD validation and active-run
checks must be coordinated with simulator commands to avoid check/write races.
Do not expose repositories directly to handlers.

Simulation's TargetStepper adapter invokes target movement models and publishes a
consistent live snapshot. Its TrafficGenerator adapter maps vessel snapshots into
AIS Reports and calls the AIS Generator. Reset AIS cadence on start/seek. The
composition root injects concrete adapters; contexts do not import each other's
infrastructure.

The viewer reads the same snapshot owner through TargetReader. It never reads a
partially updated tick. Caller-owned selection state remains in the browser.
ChartCatalog abstracts local chart metadata; browser projection is presentation.

EventBus is an optional, typed, in-process lifecycle notification port. Critical
commands and the tick-to-AIS pipeline use direct calls. No broker is required.
Subscription queues are bounded; publishers receive overflow errors and may
observe partial delivery across subscribers. Consumers resynchronize from Status.
The event adapter owns closure and unsubscription; callers never close its channels.

Each long-lived adapter has a blocking Run(context.Context) and bounded shutdown
lifecycle, satisfying app contracts structurally. Domain and application packages
never import app to declare that conformance.

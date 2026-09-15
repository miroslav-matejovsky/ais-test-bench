---
title: "05 - Public composition, thin commands, and examples"
dependencies: ["02-logging.md", "03-http-embedding.md", "04-browser-components.md"]
effort: "M"
complexity: "medium"
---

## Status

Implemented and verified on 2026-09-15.

- New public `testbench`: `New(Config{Simulation, Logger, BasePath, Tiles})`,
  `Bench.Handler` (full request paths below `BasePath`), `Bench.Run`, `Bench.UI`,
  and `Serve(ctx, ln, Config)`. The display reads the simulator in process through
  `display.New(sim)`; `internal/app` and its private loopback listener are removed.
- `simulator.DemoConfig` replaces `simdriver.NewConfig`. `simulator.Serve(ctx, ln,
  StandaloneConfig)` replaces package-level `Run` and the old `Serve(sim, handler)`;
  `NewStandaloneHandler(sim, basePath)`. `display.Serve` replaces `display.Run`;
  `StandaloneConfig` gains `BasePath` and `Tiles`. `urlpath.CheckPrefix` is the
  shared prefix rule (step 03 deferred standalone prefixes to this step).
- `httpserver.Run` is the one supervision loop: requests drain while pacing still
  runs, then pacing is cancelled and joined. Previously pacing stopped at the same
  moment as the request context, so draining writes could fail with 503.
- Commands are flags (`-addr`, `-base-path`, display `-simulator-url`), logger,
  signal context, listener, and one public `Serve` call.
- `testbench/example_test.go` has runnable examples for a host mux with middleware,
  host-template components, a display reading a prefixed remote simulator through a
  host-owned HTTP client, and two independent benches, each with a full lifecycle.
  `task consumer` (`taskfile/consumer.ps1`, part of `task all`) compiles all public
  example files in `.tmp/consumer`, an external module with a local replace
  directive, offline from the module cache.

Verification: `task all` passed (751 tests, vet, fmt, deadcode, arch-lint, lint,
consumer). `go test -race` passed for `testbench`, `internal/httpserver`, `cmd/...`,
and `display`; both drain tests passed 20 repeated runs. Tests cover construction
failure closing the listener, cancellation, request drain with a write accepted
during shutdown, serving and pacing failures stopping the other side, borrowed
clients left open, host servers surviving `Bench.Run`, logger precedence, prefix
routing and redirects, and command flags and serving. A driver failure is covered
through `httpserver.Run` with failing work, because the facade has no clock seam.
Browser checks were not rerun; no JavaScript, template, or CSS changed.

## Implementation

- Implement `testbench.New`, `Handler`, `Run`, and `Serve` using the public APIs.
  Use the local source for combined mode and remove the private loopback listener.
- Provide public standalone simulator/display serving conveniences and an explicit
  demo configuration helper. Engine config zero values retain their existing meaning.
- Reduce `cmd` to flags, logger creation, signals, listener setup, and public serving
  calls. Keep CLI validation helpers internal if useful. Remove superseded internal
  composition code and update command tests.
- Supply runnable examples for a host mux with middleware, host-template components,
  display connected to a prefixed remote simulator with a custom HTTP client, and
  two independent benches. Include imports, error handling, and complete lifecycle.
- Document request draining, driver supervision, client cleanup, listener ownership,
  and partial-start cleanup. Update command/package docs in this step.

## Verification and acceptance

- All three commands run their current manager/display workflows through published
  packages. CLI tests retain useful flag and shutdown coverage.
- Combined mode creates one engine and no private listener. Driver errors reach
  the process supervisor and stop serving through the documented shutdown path.
- Consumer examples compile as an external module using a local replace directive,
  proving they require no internal imports. Do not add a nested module that silently
  escapes normal test discovery; wire this check into repository tooling.
- Standalone shutdown tests cover construction failure, cancellation, request drain,
  and resource cleanup. Embedded handlers do not close the host's server/client.

---
title: "06 - Compose both components in the combined launcher"
dependencies: ["03-simulator-component.md", "05-standalone-display.md"]
effort: "M"
complexity: "medium"
---

## Outcome

`cmd/ais-test-bench` keeps its convenient combined entry point and public URLs,
while reusing the same simulator and display implementations as separate mode.

## Implementation work

1. Refactor `internal/app.Run` to compose component constructors and lifecycles.
   Keep `cmd/ais-test-bench -addr localhost:8080` as the normal combined command.
   There is no combined `-simulator-url`: combined mode owns its simulator.
2. Create one engine, simulator API handler, and manager handler. Bind an internal
   API listener to `127.0.0.1:0` and obtain the real bound address. Construct the
   display client with that HTTP origin, never with a simulator pointer.
3. Mount simulator API handlers on both the internal API listener and combined
   public listener. Sharing these handlers must share the same engine instance.
   Mount the manager, display handlers, homepage, status, and static assets on
   the public listener using their separate paths.
4. Bind every required listener before declaring startup successful. Register all
   handlers before starting service loops. Start the internal API server before
   accepting display traffic. Use a bound listener and explicit startup signals
   where necessary; do not add startup sleeps or health-check retry loops.
5. Keep lifecycle wiring explicit: one tick loop, two HTTP servers, one display
   HTTP client. Internal API requests must continue to be served while public
   display handlers await their responses. Do not run both sides serially on a
   single blocking execution path.
6. Treat unexpected tick-loop or HTTP server exit as failure of the combined
   application. Cancel the other component work and close all owned listeners.
   Construction or second-listener failure must release the first listener.
7. On normal cancellation, stop accepting public requests first while the internal
   API remains available for in-flight display requests. Finish or cancel that
   bounded work, close client idle connections, then shut down internal HTTP and
   stop/join the engine. Use one bounded overall shutdown policy and force-close
   remaining connections if it expires. Preserve all actionable errors.
8. Give combined pages navigation to both `/manager` and `/display`; keep `/`
   as the existing two-page entry point. Standalone root redirects remain separate
   component behavior and must not conflict on the combined router.
9. Delete the superseded monolithic UI/API handler and temporary migration paths.
   Remove browser use of engine-decoded navigation and any direct display access
   to simulator state. Move/update tests to their final owning packages.
10. If the three commands now share address parsing or server setup, extract only
    repeated concrete helpers justified by those real callers. Keep component
    policy in its owner and avoid a generic lifecycle framework.

## Verification

- Combined runtime test fetches manager HTML, display HTML, simulator metadata,
  current NMEA, and the display's decoded projection through HTTP.
- Changing count through the public simulator API changes the display result
  obtained through the internal API client path.
- Verify simulator history grows once per generated report, not once per
  listener or browser request. Combined mode must not double-start the engine.
- Test a public bind failure, an internal listener failure, an unexpected server
  exit, and cancellation while a display request is in flight.
- Verify all owned listeners close and all goroutines exit on failure/shutdown.
- Verify the combined display's HTTP client actually issues requests to the
  internal listener, using observable requests rather than concrete internals.
- Review package imports for cycles and unintended engine dependencies.

## Acceptance criteria

- Existing combined command defaults and `/manager` and `/display` remain usable.
- Separate and combined modes use identical simulator and display contracts.
- The display uses real HTTP in combined mode and cannot read engine state.
- The engine runs once and serves both API mounts consistently.
- Startup failure and shutdown leave no live component or owned listener behind.
- No temporary compatibility adapter or unused legacy handler remains.

## Effort and complexity

Medium effort and complexity. Reuse of the two component handlers is small;
startup ordering and bounded shutdown need focused tests.

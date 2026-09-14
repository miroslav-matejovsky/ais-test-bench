---
title: "05 - Add the standalone display command and browser flow"
dependencies: ["04-display-data-backend.md"]
effort: "M"
complexity: "medium"
---

## Outcome

`cmd/display` serves a live map through its own backend and connects to an
independently running simulator. An unavailable simulator does not prevent the
display page itself from opening.

## Implementation work

1. Add `cmd/display/main.go`, `doc.go`, and CLI tests. Accept `-addr`, default
   `localhost:8081`, and `-simulator-url`, default `http://localhost:8080`.
   Validate both before binding listeners. Keep the existing explicit-host
   requirement for listen addresses.
2. Add the display runtime in `internal/display`: construct the concrete HTTP
   client and handlers, serve HTTP, and shut down on cancellation. Invalid
   configuration or listener failure stops startup. An unreachable but valid
   upstream URL is handled through the page's disconnected state and retries.
3. Serve `/display`, `/display/api/vessels`, static assets, and `/` redirecting
   to `/display`. Do not mount simulator controls or start a simulation engine.
   Give the standalone page navigation that works on its own origin.
4. Point `display.js` at `/display/api/vessels`. Browser requests always go to
   the display server. The browser never fetches the simulator origin directly;
   server-side HTTP consumption therefore needs no browser CORS setup.
5. Keep one request in flight per browser. Schedule the next poll one second
   after completion. Set the browser timeout slightly above the backend's
   overall upstream deadline, for example seven seconds versus five seconds.
6. On successful response, reconcile markers against the complete active set.
   Add new vessels, update existing positions and popup fields, and remove absent
   MMSIs. For active vessels with null position, remove any obsolete marker and
   report the number without a current fix in status text.
7. Use returned spawn bounds for initial framing, especially for an empty fleet.
   Frame initial known positions at the current useful coastal zoom. Retain
   manual pan/zoom on subsequent updates. Keep the existing Show all vessels
   action, Leaflet attribution, and OpenStreetMap tile behavior.
8. Track `simulationId` in the browser. A new identity clears the previous run's
   markers and selection before applying the new complete fleet. Do not join
   tracks or identity metadata across simulator restarts with reused MMSIs.
9. On failed requests, retain last known markers, mark updates as unavailable,
   and show the last successful update time. At first load, show an empty map
   with a connecting/disconnected message. A successful empty response clears
   markers and explicitly shows zero active vessels.
10. Distinguish failure to load Leaflet/tiles from failure to fetch vessel data.
    Preserve existing useful errors and recovery behavior. Render names and type
    labels as text to keep metadata out of HTML injection paths.
11. On shutdown, stop accepting requests, let bounded in-flight work finish or
    cancel it, and close idle upstream HTTP connections. No background poller
    needs joining in this design.

## Verification

- CLI tests cover defaults, explicit simulator URL, supported schemes, malformed
  URLs, rejected URL components, invalid bind addresses, and extra arguments.
- Handler tests verify page/static routes and absence of simulator mutation routes.
- A browser check starts separate simulator and display listeners, verifies
  loaded tiles and one vessel, then changes count through the manager.
- Verify 1 to 3 to 0 to 1 count changes, popup metadata, and position updates.
- Start display before simulator, then start simulator and observe automatic
  recovery. Stop and restart simulator without reloading the display.
- Verify pan/zoom survives ordinary updates and zero vessels clears markers.
- Inspect browser requests to confirm API calls target only the display origin.

## Acceptance criteria

- `go run ./cmd/display` runs independently and does not construct an engine.
- The page connects to the configured simulator, including a different port.
- Under healthy local conditions, an update normally appears within one poll
  interval plus request time after the engine publishes it. Generation itself
  continues at its existing one-second cadence.
- Fleet changes, outages, recovery, and metadata labels work without page reload.
- A standalone simulator does not require the display process to remain alive.

## Effort and complexity

Medium effort and complexity. Reuse the existing map and poll loop. The main UI
changes are route ownership, metadata display, and explicit source restart state.

# AIS Test Bench

A local AIS simulator with a live vessel map and a manager UI. One random vessel
starts automatically in the North Sea off Rotterdam. The manager can set the fleet to 0-100
vessels. Each vessel has a stable synthetic MMSI, a name, and a random speed and
course. Positions advance once per second at the reported speed.

## Run

```text
task run
```

Open [Manager](http://localhost:8000/manager) to adjust the vessel count and inspect
recent messages. Open [Display](http://localhost:8000/display) for the live map.
Both pages see the same simulation, including changes made in other browser tabs.

The display uses Leaflet 1.9.4 and OpenStreetMap tiles. The browser needs internet
access to load Leaflet and map tiles. Application templates, CSS, JavaScript, and
htmx are embedded in the executable. No Node build is required.

```text
go run ./cmd/ais-test-bench -addr localhost:9000
task all
```

`-addr` (host:port) is the only parameter. An explicit host is required.
Development checks use the Go version in `go.mod`, Task, PowerShell,
golangci-lint, deadcode, and gotestsum.

## Architecture

One Go executable owns the simulation, in-memory state, HTTP API, and both UIs.

| Package | Responsibility |
| --- | --- |
| `internal/app` | Start the simulator and HTTP server; stop both on cancellation |
| `internal/simulation` | Random fleet, movement, synchronized snapshots, recent message history |
| `internal/ais` | Encode AIS type 1 position reports; validate NMEA with go-nmea |
| `internal/ui` | HTML pages, embedded assets, JSON API |

The simulator creates a report immediately for every new vessel and after each
one-second movement tick. Reports contain MMSI, position, speed, course, heading,
and UTC seconds, framed as checksummed `!AIVDM` sentences with CRLF. The small
encoder uses [go-nmea](https://github.com/adrianmo/go-nmea) to validate each sentence.
The simplified fixed cadence supports live development. Names are UI metadata.

The latest 1,000 reports are retained in memory, oldest first. Reducing the fleet
removes active vessels while preserving retained reports. Setting the count to
zero stops message generation. Restarting resets the fleet and all history.

The original domain/application contract subpackages and the targets, networking,
management, and visualization folders remain as design scaffolding. The running
scenario uses the four concrete packages above. TCP/UDP publishing, playback,
additional message types, and route planning are future design work.

## HTTP API

Both UIs poll once per second. JSON responses use `Cache-Control: no-store`.

| Method | Path | Response / input |
| --- | --- | --- |
| GET | `/api/vessels` | `{ "vessels": [...], "messageCount": 1, "messageLimit": 1000, "updatedAt": "..." }` |
| PUT | `/api/vessels` | Accepts `{ "count": 3 }`; returns the updated vessel snapshot |
| GET | `/api/messages` | Retained `{ "mmsi": ..., "timestamp": "...", "sentence": "!AIVDM,...\r\n" }` reports |

`PUT` requires `Content-Type: application/json` and an integer count from 0 to 100.
Malformed input returns 400; other content types return 415. A vessel includes
`mmsi`, `name`, `latitude`, `longitude`, `speed` (knots), `course` and `heading`
(degrees), and `updatedAt` (UTC). An empty fleet is an empty JSON array.

Movement follows the current speed and course over the earth's surface. Random
starting positions are offshore; this first scenario has no coastline avoidance.

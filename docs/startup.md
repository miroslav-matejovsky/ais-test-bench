# Startup and shutdown

## Running

```text
task run                                          # http://localhost:8080
go run ./cmd/ais-test-bench -addr localhost:9000  # http://localhost:9000
```

`-addr` is the only command-line parameter, a `host:port` listen address. It
defaults to `localhost:8080`. The host is required: an empty host such as
`:8080` binds all interfaces, which triggers Microsoft Defender firewall prompts.
The port must be 1-65535.

## Sequence

1. `cmd/ais-test-bench` parses and validates `-addr`.
2. It creates a context cancelled on SIGINT or SIGTERM.
3. It binds the listener, so a busy port fails before anything else starts.
4. `app.Run` builds the `ui` handler, which parses the shared templates. A
   template error stops startup.
5. `app.Run` serves HTTP until the context is cancelled or serving fails.
6. On cancellation it calls `http.Server.Shutdown` with a 5 second budget,
   detached from the cancelled context, and returns nil after a clean stop.

## Routes

| Route | Behavior |
| --- | --- |
| GET / | Home page linking both UIs |
| GET /manager | Manager UI: scenarios, targets, simulator control (placeholder) |
| GET /display | Display UI: live vessel view (placeholder, polls /status every 2s) |
| GET /status | Server time and uptime; fragment for htmx partial requests, full page otherwise |
| GET /static/ | Embedded CSS and vendored htmx |

## Rendering

Templates follow the layout from
[How I use HTMX with Go](https://www.alexedwards.net/blog/how-i-use-htmx-with-go),
adapted to htmx 4:

- `html/base.tmpl` is the shared layout. Each `html/pages/*.tmpl` file defines
  `page:title`, `page:content`, and fragments used only by that page.
- A handler renders `base` for a full page, or one fragment when the request has
  `HX-Request-Type: partial`. htmx 4 sends `HX-Request: true` on every request,
  including history restores, so `HX-Request` alone cannot select a fragment.
- Every rendered response sets `Vary: HX-Request-Type`.
- htmx 4 defaults are used as-is: explicit attribute inheritance, history restore
  from the server, and 4xx/5xx responses swapped into the target.

## Planned wiring

As bounded contexts gain implementations, `app.Run` constructs them and passes
use cases into `ui`. Long-lived workers (simulation, TCP, UDP) will run beside the
HTTP server under one cancellation context, and shutdown will stop HTTP
admission, stop simulation, then drain publisher queues within a finite budget.
TCP and UDP use bounded queues; one slow client must not stall virtual time.

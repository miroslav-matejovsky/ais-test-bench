# Dependencies

Runtime code uses the standard library only. Add and pin libraries when their
adapters are implemented; keep third-party types outside domain contracts.

| Choice | Status | Purpose and boundary | Primary source |
| --- | --- | --- | --- |
| net/http, html/template, embed, io/fs | Used | HTTP serving, server-rendered UI, embedded assets | [Go standard library](https://pkg.go.dev/std) |
| context, os/signal, log/slog, flag | Used | Cancellation, process signals, structured logs, `-port` flag | [Go standard library](https://pkg.go.dev/std) |
| htmx 4.0.0 | Used, vendored | Partial page updates; `internal/ui/assets/static/js/htmx.min.js` | [htmx 4 docs](https://four.htmx.org/docs/) |
| stretchr/testify/require | Used in tests | Assertions | [require](https://pkg.go.dev/github.com/stretchr/testify/require) |
| golang.org/x/sync/errgroup | Planned | Supervise HTTP, simulation, TCP, UDP workers with error propagation | [errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup) |
| prometheus/client_golang | Planned | Private metrics registry and /metrics handler | [Official Go client](https://github.com/prometheus/client_golang) |

To update htmx, download `dist/htmx.min.js` for the new version from
`https://cdn.jsdelivr.net/npm/htmx.org@<version>/dist/htmx.min.js` into
`internal/ui/assets/static/js/` and update the version above.

Prefer net/http.ServeMux. Keep dependency injection explicit in app. Use local
file repository adapters initially, with atomic replacement and clear
single-process write ownership. A database can be introduced behind repository
ports if scenario storage requirements justify it.

Select an AIS codec only after supported message types and known-vector tests are
defined. Wrap it behind AISEncoder. Select a chart library after choosing local
chart formats and required offline behavior.

# Recommended dependencies

The scaffold builds with the standard library only. Add and pin libraries when
their adapters are implemented; keep third-party types outside domain contracts.

| Choice | Purpose and boundary | Primary source |
| --- | --- | --- |
| net/http, embed, io/fs | REST routing, HTTP serving, embedded UI assets | [Go HTTP](https://pkg.go.dev/net/http), [embed](https://pkg.go.dev/embed) |
| context, os/signal, log/slog | Cancellation, process signals, structured logs | [Go standard library](https://pkg.go.dev/std) |
| golang.org/x/sync/errgroup | Supervise HTTP, simulation, TCP, UDP workers with error propagation | [errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup) |
| go.yaml.in/yaml/v3 | Strict configuration decoding in the loader | [YAML documentation](https://pkg.go.dev/go.yaml.in/yaml/v3) |
| prometheus/client_golang | Private metrics registry and /metrics HTTP handler | [Official Go client](https://github.com/prometheus/client_golang) |
| stretchr/testify/require | Concise assertions once behavior has tests | [require](https://pkg.go.dev/github.com/stretchr/testify/require) |
| React | Independent admin and viewer builds using the JSON API | [React](https://react.dev/learn) |
| HTMX + Templ, alternative | Server-rendered presentation handlers with embedded static assets | [HTMX](https://htmx.org/docs/), [Templ](https://templ.guide/) |

Prefer net/http.ServeMux initially. Keep dependency injection explicit in app.
Use local file repository adapters initially, with atomic replacement and clear
single-process write ownership. A database can be introduced behind repository
ports if scenario storage requirements justify it.

Select an AIS codec only after supported message types and known-vector tests are
defined. Wrap it behind AISEncoder. Select a chart library after choosing local
chart formats and required offline behavior. Neither choice is needed to establish
these boundaries.

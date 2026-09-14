# Configuration

`config.yaml` is the example startup configuration. The intended default lookup
is `./config.yaml`; an explicit `-config` flag selects another path. The current
scaffold does not load configuration.

Schema: [config.go](../internal/app/config/config.go). Planned load order:
built-in defaults, then YAML overrides. Reject unknown keys, duplicate keys,
extra YAML documents, invalid durations, and invalid values before opening sockets.
An explicitly selected missing file is an error. Duration strings use Go syntax.

Validate positive tick intervals, queue capacities, client/body limits, finite
positive playback speed, and finite positive HTTP/shutdown/write budgets.
Validate enabled transport addresses and ports; enabled UDP requires at least
one destination. Broadcast requires explicit opt-in and IPv4 destinations.
Validate logging enums, writable data directory, and autostart scenario references.
Disabled transports need no valid destination settings.

Use loopback defaults. Configuration is immutable during a run; changing listeners
requires restart. Management exposes a detached effective configuration snapshot.

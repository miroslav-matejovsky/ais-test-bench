# Commands

Each executable validates its flags before listening (`-addr` is host:port with
an explicit host), binds its public listener, and delegates serving and shutdown
to its internal package. SIGINT and SIGTERM stop it gracefully.

| Command | Flags and defaults | Runs |
| --- | --- | --- |
| `ais-test-bench/` | `-addr localhost:8000` | `internal/app`: one engine, simulator API, manager, and display in one process |
| `simulator/` | `-addr localhost:8000` | `internal/simulator`: one engine, simulator API, and manager |
| `display/` | `-addr localhost:8081`, `-simulator-url http://localhost:8000` | `internal/display`: display page and backend reading the simulator over HTTP |

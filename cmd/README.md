# Commands

Each executable validates its flags before listening (`-addr` is host:port with
an explicit host; `-base-path` is an optional route prefix), creates the stderr
logger and signal context, binds its public listener, and calls one public
serving function. SIGINT and SIGTERM stop it gracefully.

| Command | Flags and defaults | Runs |
| --- | --- | --- |
| `ais-testbench/` | `-addr localhost:8000`, `-base-path ""` | `testbench.Serve` with `simulator.DemoConfig`: one engine, simulator API, manager, and display reading the engine in process |
| `simulator/` | `-addr localhost:8000`, `-base-path ""` | `simulator.Serve` with `simulator.DemoConfig`: one engine, simulator API, and manager |
| `display/` | `-addr localhost:8081`, `-simulator-url http://localhost:8000`, `-base-path ""` | `display.Serve`: display page and backend reading `{simulator-url}/api/` over HTTP and linking `{simulator-url}/manager` |

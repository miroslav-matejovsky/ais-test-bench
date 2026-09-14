# Commands

`ais-test-bench/` is the sole executable. It parses the `-addr` flag, listens on
that host:port, and delegates serving and shutdown to `internal/app`.

# Deployment

Package one `ais-test-bench` executable with embedded admin and viewer bundles,
an example config, and a writable data directory. HTTP, REST, both UIs, TCP, and
UDP run in that same process.

The initial deployment target is a local workstation. Loopback listeners and
explicit UDP destinations are defaults. Future service/container definitions
must preserve the same process model, deliver shutdown signals, and allow the
configured shutdown timeout. No RF hardware or RF transmission is involved.

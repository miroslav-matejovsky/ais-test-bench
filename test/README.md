# Test strategy

Unit tests live beside the Go package they exercise as `*_test.go`.
Use table-driven cases and `testify/require` when behavior is implemented.
This scaffold contains contracts and asset declarations; behavioral tests arrive
with implementations.

`integration/` covers assembled adapters and transport boundaries.
`simulation/` covers deterministic scenarios and playback.
`testdata/` holds small fixtures shared by those suites.

Avoid sleeps. Drive clocks explicitly, use synchronization for readiness, and bind
loopback port 0 for socket tests. Context deadlines bound failures. Include race
checks when concurrent workers are introduced.

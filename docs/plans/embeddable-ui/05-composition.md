---
title: "05 - Public composition, thin commands, and examples"
dependencies: ["02-logging.md", "03-http-embedding.md", "04-browser-components.md"]
effort: "M"
complexity: "medium"
---

## Implementation

- Implement `testbench.New`, `Handler`, `Run`, and `Serve` using the public APIs.
  Use the local source for combined mode and remove the private loopback listener.
- Provide public standalone simulator/display serving conveniences and an explicit
  demo configuration helper. Engine config zero values retain their existing meaning.
- Reduce `cmd` to flags, logger creation, signals, listener setup, and public serving
  calls. Keep CLI validation helpers internal if useful. Remove superseded internal
  composition code and update command tests.
- Supply runnable examples for a host mux with middleware, host-template components,
  display connected to a prefixed remote simulator with a custom HTTP client, and
  two independent benches. Include imports, error handling, and complete lifecycle.
- Document request draining, driver supervision, client cleanup, listener ownership,
  and partial-start cleanup. Update command/package docs in this step.

## Verification and acceptance

- All three commands run their current manager/display workflows through published
  packages. CLI tests retain useful flag and shutdown coverage.
- Combined mode creates one engine and no private listener. Driver errors reach
  the process supervisor and stop serving through the documented shutdown path.
- Consumer examples compile as an external module using a local replace directive,
  proving they require no internal imports. Do not add a nested module that silently
  escapes normal test discovery; wire this check into repository tooling.
- Standalone shutdown tests cover construction failure, cancellation, request drain,
  and resource cleanup. Embedded handlers do not close the host's server/client.

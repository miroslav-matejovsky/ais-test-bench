---
title: "06 - Integration verification and documentation"
dependencies: ["05-composition.md"]
effort: "M"
complexity: "medium"
---

## Status

Implemented on 2026-09-15. Manual UI verification by the maintainer is pending;
remove these plans after it is accepted.

- Docs synchronized: root README (run table, architecture, composition, embedding,
  verification), `cmd/README.md`, `internal/README.md`, `taskfile/README.md`,
  `docs/simulation.md`, package docs, and `.go-arch-lint.yml`. No links to removed
  internal packages remain.
- Combined integration tests moved from `internal/app` to `testbench`.
  `BenchmarkDisplayObservations` still measures the separate-process HTTP path,
  now documented as the slowest path; payload limits are unchanged.
- Public API review: `go doc -all` of `simulation`, `simulator`, `simulatorapi`,
  `display`, `ui`, and `testbench` exposes no internal types. New
  `TestAssetsHaveNoApplicationPaths` keeps templates and browser modules free of
  hard-coded application routes. All three commands call only public `Serve`
  functions.
- `task consumer` runs in `task all`. Browser checks stay manual
  (`ui/testdata/README.md`).

Verification matrix, covered by automated tests unless noted:

| Case | Coverage |
| --- | --- |
| Manager only / display only | `simulator` and `display` standalone route tests, `cmd/*` serve tests, `ui` disabled component test |
| Combined | `testbench` handler, Serve, and command tests |
| Host-template embed | `ui` and `testbench` examples, `ui` component ID tests |
| Root, nested, proxy prefixes | `simulator/embedding_test.go`, base path tests in `simulator`, `display`, `testbench` |
| Remote API prefix | `TestRemoteSourceReadsPrefixedAuthenticatedSimulator`, `Example_remoteDisplay` |
| Multiple instances | `embedding_test.go`, `Example_independentBenches`, host browser fixture |
| Injected logger | `simulator`, `display`, `httpserver`, `testbench` logging tests |
| Protected writes | `TestHostMiddlewareProtectsEmbeddedHandlers`, `Example` |
| Cancellation and drain | `httpserver.Run`, `simulator`, `display`, `testbench` Serve tests |
| Offline map fallback | Browser check `display-browser.mjs` (manual) |

Verification: `go test -race ./...` passed for all packages. `task all` passed
(752 tests, vet, fmt, deadcode, arch-lint, lint, consumer). Browser checks were
not run in this step.

## Implementation

- Update root README architecture, public Go API, URL tables, examples, logging,
  ownership, and local/remote data paths. Replace obsolete internal-package links.
- Synchronize `.go-arch-lint.yml`, `internal/README.md`, `cmd/README.md`, package docs,
  assets documentation, browser-check instructions, and relevant task definitions.
- Adapt combined integration/benchmark tests that currently assume a private
  listener. Preserve payload limits and existing reception/validation coverage.
- Make external-consumer and deterministic browser checks explicit required
  verification for this refactor. Keep the existing no-frontend-build workflow.

## Verification and acceptance

- Run `task all` during implementation and require format, lint, architecture, and
  tests to pass. This requirement does not apply to the current planning-only task.
- Run the documented browser checks and external-consumer compile check; record
  exact commands and outcomes. Run the race detector where the supported toolchain
  permits it, focusing on runtime, shared handlers, and instance isolation.
- Complete the matrix: manager only, display only, combined, host-template embed,
  root/nested/proxy prefixes, remote API prefix, multiple instances, injected logger,
  protected writes, cancellation, and offline map fallback.
- Review every exported signature for internal types and every browser route for
  hard-coded application paths. Confirm all commands use the published packages.
- Remove this plan only after all criteria are verified and implementation accepted.

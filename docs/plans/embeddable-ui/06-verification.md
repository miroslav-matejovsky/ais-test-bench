---
title: "06 - Integration verification and documentation"
dependencies: ["05-composition.md"]
effort: "M"
complexity: "medium"
---

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

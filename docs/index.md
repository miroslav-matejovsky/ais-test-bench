# Docs

Entry point for this repository's documentation. Start with the root
[`README.md`](../README.md) for how to run the applications, the HTTP APIs,
and the Go package contract.

## Contents

- [`simulation.md`](simulation.md) - what the AIS simulation supports and why,
  with references to the real-world scenarios it models.
- [`plans/`](plans/README.md) - active implementation plans.
- [`backlog/`](backlog/README.md) - optional enhancements not yet planned.
- [`bugs/`](bugs/README.md) - observed defects awaiting a fix.
- [`assessments/`](assessments/README.md) - impact/feasibility assessments.
- [`evaluations/`](evaluations/README.md) - evaluation notes.

Package-level behavior is documented next to the code: run `go doc -all
./simulation`, `go doc -all ./internal/simdriver`, or the equivalent for any
other package under `internal/`.

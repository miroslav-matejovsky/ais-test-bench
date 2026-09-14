---
title: "05 - Verify external use and complete documentation"
dependencies: ["01-public-package.md", "02-virtual-time.md", "03-runtime-api.md", "04-manager-display.md"]
effort: "M"
complexity: "medium"
---

## Outcome

External Go use, deterministic time behavior, both application modes, and the
required development checks are demonstrated and documented.

## Implementation work

1. Add runnable `simulation_test` examples for deterministic initial creation,
   complete message batches, virtual stepping, elapsed-duration scaling, and
   pause/resume. Handle every error; examples need no server, network, or sleep.
2. Add a concise public-package usage section to root `README.md`, with import
   path, explicit config, actual returned messages, time units, speed limits,
   history loss/gap detection, batching, and cancellation/atomicity semantics.
   Document caller responsibility for real pacing and ordered commands.
3. Update architecture diagram/table, simulator and display API examples,
   timestamp descriptions, reporting cadence, and UI controls. Keep old scaffold
   docs scoped to their own contracts and avoid implying they are implemented.
4. Document live catch-up limits, the 100 ms real heartbeat, one-second virtual
   report cadence, separate snapshot consistency, and the distinction between
   fixed vessel knots and adjustable virtual-time speed.
5. Review deadcode output for externally intended public methods. If needed,
   extend `taskfile/deadcode.ps1` with exact, documented external API symbols
   justified by executable examples and external-module verification. Do not
   ignore the whole public package or weaken internal dead-code checks.
6. Use a temporary independent Go module with a local `replace` to this checkout
   and the same required Go toolchain. Import the public package, construct a
   simulator, advance time, set speed, and consume messages. Build/run it without
   importing any `internal` path. Use cached dependencies or record any network
   prerequisite; the library must not rely on the consumer using this checkout's
   vendor mode. Do not leave the temporary module in the source tree.
7. Run focused checks, then the required `task all` once the implementation is
   ready. Inspect any formatting/tidy changes. Keep `vendor` read-only and do not
   add dependencies as part of this plan. Never commit.

## Verification and acceptance matrix

| Requirement | Evidence required |
| --- | --- |
| Importable library | Independent-module build/run plus executable public examples. |
| No server requirement | Example generates valid AIS messages without listeners, HTTP setup, or UI imports. |
| One implementation | App and standalone integration tests call the public engine through the thin driver; no duplicate generator remains. |
| Reproducibility | Identical ordered scenarios produce identical messages, snapshots, and clock state across chunk sizes. |
| Custom time | Fixed past/future UTC start dates propagate to creation and scheduled AIS/envelope timestamps. |
| Speed | Manual elapsed inputs and fake-clock driver tests cover slow/fast/pause/change boundaries without sleeping. |
| Complete output | Large permitted batch returns all reports while history retains only its configured capacity. |
| API | Strict write validation, read-only metadata, time units, restart consistency, and error paths pass. |
| UI | Combined and separate modes pass the step 04 interaction checklist, including two tabs and reconnect. |
| Defects | B1/B2 regression tests pass; B3 controlled-response browser verification passes. |
| Concurrency | Race checks for engine, driver, simulator, app, and display pass; lifecycle tests detect leaks through explicit joins. |
| Load | Exercise 100 vessels at the advertised maximum speed with a bounded run; capture generation throughput, API responsiveness, and bounded retained memory. |
| Quality | `task all` passes with tests, format, vet, deadcode, and lint. |
| Documentation | Public APIs, package docs, examples, architecture, and wire contracts match final behavior. |

Load verification is a targeted acceptance experiment, not a new benchmark
framework or timing-sensitive unit test. At 100 vessels and 100x, the expected
generation rate is 10,000 reports per real second, and 1,000 retained reports
cover only about 0.1 real seconds. Measure whether generation keeps up and reads
remain within existing five-second deadlines. If it cannot, lower the maximum
consistently in Go validation, metadata, UI, docs, and tests before acceptance.
Do not silently drop due reports to claim the higher rate is supported.

## Completion

The work is complete when every matrix row has evidence and all planned bug
fixes are implemented. Record any genuinely remaining implementation or manual
verification work in root `.todo`; do not report completion while it remains.
Retire this plan only after all acceptance criteria are verified and the work is
accepted, as specified by `docs/plans/README.md`.

## Estimate

Approximately 1-2 days, partly overlapping documentation and verification in
earlier steps. Main risks: external-module dependency resolution, exported API
handling in deadcode, and final load behavior at maximum speed.

---
title: "07 - Verify all runtime modes and update operating documentation"
dependencies: ["01-api-contracts.md", "02-engine-state-and-metadata.md", "03-simulator-component.md", "04-display-data-backend.md", "05-standalone-display.md", "06-combined-launcher.md"]
effort: "M"
complexity: "medium"
---

## Outcome

The implemented split is verified as one working scenario, and documentation
explains how to run the components and consume their APIs.

## Implementation work

1. Update the root `README.md` with the two-component architecture, actual package
   ownership, three commands, defaults, URLs, and the real HTTP data path in both
   modes. Explain that manager belongs to simulator and display has its own backend.
2. Document the simulator and display APIs near their owning packages. Include
   complete response examples captured from verified fixtures, count commands,
   metadata semantics, NMEA CRLF handling, history retention, sequences, and
   restart behavior. Keep examples usable by ordinary HTTP clients.
3. Update `cmd/README.md` and all new/changed `doc.go` files. Update UI/assets docs
   to describe route ownership and external Leaflet/tile dependencies. Remove
   claims that the combined executable is the sole entry point.
4. Keep `task run` for the combined command. Add `task run:simulator` and
   `task run:display` with documented standalone defaults. Do not add deployment
   tooling or a process supervisor. Show separate-terminal use in the README.
5. Adapt existing unit tests to final package ownership. Use table-driven tests
   and `testify/require` where appropriate. Keep deterministic unit tests for
   state/codec behavior and a small set of HTTP/runtime integration tests.
6. Run the acceptance matrix below. Use controlled listeners, channels, explicit
   time, and fixture servers; avoid arbitrary sleeps or fixed test ports.
7. Once implementation is complete, run `task all`. Also run focused race tests
   for changed state, HTTP, and lifecycle packages, and browser checks for the
   live flow. A final plan-authoring change alone does not run these commands.
8. Record any incomplete implementation work in the repository `.todo`. Remove
   completed migration notes. Do not mark the implementation complete while a
   required mode or check fails. Never commit changes.

## Acceptance matrix

| Scenario | Expected result |
| --- | --- |
| Simulator alone | One engine, manager usable, metadata and NMEA readable |
| Display alone with unavailable upstream | Page opens, useful disconnected state, bounded retrying requests |
| Two standalone processes | Display derives live positions from simulator NMEA |
| Combined process | Same live flow and public URLs; internal HTTP boundary remains |
| Independent external client | Reads messages and metadata without UI-specific dependencies |
| Default startup | One random offshore vessel, immediate AIS report, visible marker |
| Count 1 -> 3 -> 0 -> 1 | Correct active set without reload; history preserved within its limit |
| Movement | Decoded position changes consistently with generated AIS data |
| Metadata | Type labels and effective settings match engine behavior |
| Invalid count payload | Defined error, no partial state change |
| History reaches capacity | Oldest reports evicted, active report snapshot remains complete |
| Simulator restart | New run identity; display replaces reused MMSIs without mixing runs |
| Simulator outage and recovery | Last known map marked unavailable, automatic complete recovery |
| Invalid upstream AIS or metadata | Contextual error, no false zero-position marker or partial fleet |
| Several display clients | Shared engine unaffected; independent browser controls |
| Component shutdown | Owned resources close; other standalone process remains independent |
| Combined startup/shutdown failure | All owned components cleaned up and useful error returned |
| Map asset failure | Clear map-library/tile feedback separate from simulator connectivity |

## Verification evidence

Record successful commands and a short browser checklist in the implementation
handoff. Include any limitations observed. Verify `task all` reports passing
tests, format, vet, lint, and dead-code checks. Do not substitute a build-only
result for the full required checks.

## Acceptance criteria

- Every matrix row is verified or explicitly reported as unfinished work.
- All three commands and Task entries work as documented.
- Repository documentation matches implemented routes, data, and ownership.
- `task all` passes after the final implementation edits.
- Race tests pass for changed concurrent code and browser checks confirm the
  complete manager-to-simulator-to-display path.

## Effort and complexity

Medium effort and complexity. Most tests are added in their implementation
steps; this step verifies their integration and updates operating documentation.

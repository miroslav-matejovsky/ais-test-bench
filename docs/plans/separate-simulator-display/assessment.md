# Impact and feasibility assessment

## Summary

The split is feasible with the current Go standard-library HTTP stack, existing
templates/JavaScript, and go-nmea dependency. The engine already owns the core
simulation behavior. Most work is separating HTTP ownership and making the
display consume encoded reports through a real client.

Overall implementation effort is large relative to the current small codebase,
with medium technical complexity. AIS decoding, consistent state snapshots,
and combined lifecycle behavior need the most verification. No new database,
message broker, frontend build system, or service framework is required.

## Impact by area

| Area | Impact | Reason |
| --- | --- | --- |
| Simulation movement | Low | Preserve current position calculation and cadence |
| State representation | Medium | Add run identity, type ID, report sequence, and per-vessel latest report |
| AIS codec | Medium | Add supported-report decoding and independent fixtures |
| Simulator API | High | Extract ownership and replace decoded fleet data with NMEA snapshots |
| Metadata | Medium | Expose real settings and a small supported type catalog |
| Manager | Low to medium | Adapt response envelopes and metadata-driven labels/limits |
| Display backend | High | New HTTP consumer and decoding/projection path |
| Display browser | Medium | Use its backend route, metadata, and source restart/error behavior |
| Commands and lifecycle | Medium | Two independent commands and combined reuse |
| Documentation and tests | Medium | Explain and verify three runtime modes and new API semantics |

## Risks and mitigations

| Risk | Practical mitigation | Verification |
| --- | --- | --- |
| Shared AIS encoder/decoder repeat the same bit mistake | Include an independent known NMEA fixture and exact expected fields | Step 04 codec tests |
| Display still depends on engine structs | Keep shared types transport-only; require HTTP client and inspect imports | Steps 04 and 06 |
| History replay revives removed vessels | Use a complete active-report snapshot separate from history | Steps 02 and 04 |
| Restart reuses MMSIs and mixes metadata | New run identity; compare upstream identities and reset browser run state | Steps 04 and 05 |
| Snapshot mixes report and navigation updates | Publish state changes atomically and return locked copies | Step 02 |
| Metadata becomes a second configuration source | Derive response values from actual generation settings/constants | Steps 02 and 03 |
| Upstream failure hangs the display | Request-derived contexts, one overall deadline, bounded bodies, sequential browser polling | Steps 04 and 05 |
| Corrupt report silently removes or misplaces a vessel | Reject the projection; distinguish unavailable position from invalid data | Step 04 |
| Combined mode starts two engines | One shared simulator instance and reusable API handler | Step 06 |
| Combined startup leaks a listener or shutdown breaks in-flight reads | Explicit ownership, rollback on failure, ordered bounded shutdown | Step 06 |
| Standalone pages link to nonexistent local routes | Mode-specific navigation and isolated route sets | Steps 03 and 05 |
| Full polling increases traffic with many displays | Keep current 100-vessel bound; document request-driven polling before adding optimizations | Step 07 |

## Chosen tradeoffs

### Complete snapshots and request-driven display

A complete latest-report snapshot makes new clients, fleet removals, and recovery
simple. The display backend does two bounded reads per browser refresh, validates
the run identities, and returns a decoded snapshot. This avoids background-worker
lifecycle and a second stored projection. Repeated decoding is acceptable for
the current local workload. Optimize only if a measured workload requires it.

History remains useful for inspection and external polling. It has report
sequences for deduplication and gap detection, but it is not a guaranteed-delivery
feed. No cursor or durable replay is needed for the requested live scenario.

### One combined public listener and an internal API listener

Combined mode keeps existing URLs under one public address and uses a private
loopback listener for the display's real HTTP consumption. This adds a small
resource-ownership cost. It avoids an in-process bypass and does not require
users to configure a second public port when using the combined command.

Both API mounts share one engine. The private address is discovered after bind,
so wildcard public addresses or port changes are not mistaken for client URLs.

### Metadata describes implemented behavior

The initial type catalog can have one application category. Settings expose the
existing limits, region, and timing. Count remains the only mutable control.
There is no requirement to create settings persistence, a generic metadata
registry, or extra AIS message types merely to populate the metadata endpoint.

### Design-phase API changes

The current `/api/vessels` and `/api/messages` response shapes change. Existing
browser scripts and tests must migrate with them. The repository is in design
phase, so the final result keeps one contract rather than carrying compatibility
routes. External examples must clearly describe the new finite NMEA API.

## Assumptions and limits

- Both components remain in one Go module and may share stateless code.
- Independent execution means each has its own command, listener, and lifecycle;
  it does not require separate repositories or separate release pipelines.
- Real-time means the existing one-second generation and polling scenario,
  allowing HTTP latency. This is not a hard real-time system.
- Local development is the initial deployment context. Listen hosts remain
  explicit; production access control and TLS termination are separate work.
- Leaflet and map tiles continue to require browser internet access.
- Simulator restart resets in-memory history and creates a new run identity.
- The display supports the AIS position messages generated by this simulator.
  Arbitrary third-party AIS feeds and multipart static data are later work.

## Completion decision

Accept the implementation only after the matrix in
[step 07](07-acceptance-and-documentation.md) is satisfied and required checks
pass. This planning change does not claim those implementation criteria have
already been met and does not run `task all`.

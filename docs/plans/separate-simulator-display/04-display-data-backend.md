---
title: "04 - Build the display HTTP client and AIS projection"
dependencies: ["01-api-contracts.md", "03-simulator-component.md"]
effort: "L"
complexity: "medium"
---

## Outcome

The display backend fetches the public simulator contract over HTTP and derives
map positions from NMEA. It works without importing or reading engine state.

## Implementation work

1. Add a small type 1 position decoder to `internal/ais`. Use go-nmea to parse
   and validate NMEA framing, checksum, and the armored payload. Decode the
   supported payload fields explicitly. The existing dependency exposes VDM/VDO
   payload bits; using it does not itself decode all AIS navigation fields.
2. Limit initial support to the single-fragment AIVDM type 1 reports generated
   by this engine. Validate fragment count, payload length, message type, MMSI,
   signed coordinates, ranges, and field availability. Return contextual errors
   for unsupported reports. Do not add speculative multipart assembly.
3. Map AIS unavailable sentinels to explicit nullable navigation fields. A report
   without an available position must not produce a marker at `(0, 0)`. Keep its
   identity in the browser projection with null coordinates so the browser can
   distinguish an active vessel without a fix from a removed vessel.
4. Create a concrete simulator HTTP client in `internal/display`. Its configured
   base URL is a server origin with HTTP or HTTPS, a nonempty host, and optional
   port. Allow an empty path or `/`; reject user info, query, fragment, and other
   paths in this first version. Never take the upstream URL from a browser request.
5. On `GET /display/api/vessels`, fetch `/api/vessels` and `/api/metadata` using
   request-derived contexts. The two reads may run concurrently with one overall
   five-second upstream deadline. Use one reusable HTTP client, close response
   bodies, check status/content type, and bound each decoded body to 2 MiB.
   There are at most 100 current reports, so this leaves ample room.
6. Validate both responses before producing a projection. Their `simulationId`
   values must match. Validate required metadata fields and unique vessel MMSIs.
   Decode each sentence and check its MMSI against the envelope. Keep name/type
   assignment from the fleet snapshot and enrich type labels from metadata.
7. Publish one complete projection only after all required data validates. A
   malformed report returns 502 for that request, preserving the browser's last
   successful fleet. Do not silently skip corrupt reports and accidentally make
   the browser interpret them as fleet removals. Unknown type labels can use the
   supplied `typeId` as text; invalid navigation is not a labeling fallback.
8. Map upstream connection failures, timeouts, or inconsistent run identities to
   503; invalid upstream content to 502. Include useful operation context in
   logs and concise messages to the browser. Browser cancellation cancels both
   upstream reads. No request handler launches work that survives its request.
9. Return `simulationId`, source `updatedAt`, decoded vessels, and map-relevant
   metadata as specified in step 01. The full generation timestamp comes from
   the report envelope; do not invent a date from the AIS UTC-second field.
10. Keep this backend request-driven. It does not own a tick loop, history store,
    shared vessel projection cache, or a second simulation. The browser's
    existing poll loop controls refresh, even in combined mode.

## Verification

Codec tests must include an independently specified known report, not only
encoder/decoder round trips. Extend the existing AIS test style with:

- Positive and negative coordinates, field boundaries, expected MMSI, speed,
  course, heading, and timestamp seconds.
- AIS precision tolerances, signed-coordinate interpretation, unavailable
  sentinels, corrupt checksum, wrong payload length, wrong message type, and
  unsupported multipart input.
- Round trips with the existing encoder as additional coverage.

Use `httptest.Server` for display client and handler tests:

- A fixture containing NMEA and identity data, with no decoded coordinates,
  produces the expected browser coordinates and metadata label.
- Changing only the NMEA position changes the returned map position.
- Multiple readers do not mutate simulator data or one another's responses.
- Empty fleet succeeds without reading history or decoding nonexistent reports.
- History containing removed vessels has no influence on the display result.
- Engine restart with reused MMSIs changes `simulationId` and replaces the fleet.
- Mismatched run identities between the two reads return retryable 503.
- Unreachable upstream, non-success status, invalid JSON/content type, oversized
  body, invalid metadata, duplicate MMSI, and invalid NMEA produce defined errors.
- Cancellation propagates to in-flight reads. Use channels or controlled
  transports for deadline/cancellation cases instead of timing sleeps.

## Acceptance criteria

- Display packages import neither `internal/simulation` nor the simulator runtime.
- Navigation comes exclusively from decoded report payloads.
- The display uses the documented simulator API and can be tested against an
  independent HTTP fixture server.
- A failed read cannot publish a misleading empty or partial fleet.
- Timeouts and size limits bound the request work; all response bodies close.

## Effort and complexity

Large relative effort, medium complexity. AIS field decoding and its independent
test fixtures require the most care. HTTP consumption stays small and concrete.

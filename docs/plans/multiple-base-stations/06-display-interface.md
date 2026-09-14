---
title: "06 - Coverage map and station, signal, and target details"
dependencies: ["05-display-backend.md"]
effort: "L"
complexity: "high"
---

# Coverage map and station, signal, and target details

## Outcome

Users can compare receiving sites, see estimated coverage and capabilities, and
inspect which AIS signals and target updates each station actually received.

## Implementation work

Extend the existing display template, `display.js`, and shared CSS. Keep Leaflet
and the embedded asset approach. Use plain DOM rendering and focused functions;
add no frontend framework or Node build requirement.

### Layout

```text
Simulation UTC | speed / pause | browser last fetched | connection state
Stations: [All] [Rotterdam coast] [Northern coast] [Harbour receiver]
Coverage channel: [A] [B]   Layers: [coverage] [stations] [lost targets]
+------------------------------------+----------------------------------+
|                                    | Selected station / target        |
| Map: sites, coverage, AIS targets   | Summary, capabilities, details   |
|                                    |                                  |
+------------------------------------+----------------------------------+
Station comparison table | Observed targets | Recent received messages
```

Collapse the side panel below the map on narrow screens. Preserve selection,
open details, map zoom, and scroll across polls. Use stable station ID as the
selection key and stable labels/colors for the duration of a simulation run.

### Map and coverage

- Distinct station icon with name, short ID, administrative state, and active
  A/B channel badges. Disabled sites use a muted icon with an explicit text label.
- Render simulator-supplied 90% and 50% coverage contours in the station color.
  Use a filled inner area and dashed outer outline, low opacity, and a clear
  legend. State "Estimated per-message reception, reference transmitter" with
  power and antenna height accessible beside the legend.
- Select A or B coverage separately to avoid obscuring two almost identical
  polygons. Different disabled/impaired channels remain visible in the capability
  table. Do not draw a receive area for a disabled site/channel.
- Clicking a station selects its received-target view, emphasizes its coverage,
  and opens station details. Multi-selection shows the union of received targets.
  Hiding a coverage layer changes presentation only, not target selection or RF.
- Render a single marker per aggregate target MMSI using the chosen received
  report. Stale targets are muted; lost targets are hidden by default and can be
  shown with an explicit last-known style. No predicted movement between reports.
- Optional selected-target lines connect it to stations that received the chosen
  transmission. Label their receipt times; do not animate every reception at high
  speed. A marker is a last-known position, not proof of current RF visibility.
- Initial fit includes station locations and received targets, with spawn bounds
  as fallback. Offer "Fit coverage" separately because VHF coverage is much wider
  than the existing spawn area. Never auto-fit on every poll.

### Station comparison and details

The comparison table presents name/ID, status, active channels, antenna height,
sensitivity A/B, gain/loss summary, fresh/stale/lost target counts, recent received
count, opportunity ratio, and last received time. Sorting and text filtering do
not alter simulation state. Capability comparisons use unit labels rather than
an invented single "receiver quality" score.

The selected station panel includes:

| Group | Fields |
| --- | --- |
| Identity and state | Name, ID, location, enabled/degraded state, config and RF revision. |
| Site capability | Antenna height above sea level, gain dBi, feeder loss dB, shadow-sector bearings/losses. |
| Channel capability | A/B frequencies, enabled state, sensitivity dBm, noise penalty dB, extra packet-drop probability. |
| Coverage assumptions | Published model parameters, reference transmitter, threshold legend, min/max contour range in nautical miles and km. |
| Reception summary | Counts since creation, 60-second virtual window and actual duration, per-channel successes, simulated opportunity ratio, outcome reasons. |
| Observed targets | MMSI, scenario name, last received time, virtual age/status, last channel/power, received position/speed/course/heading. |
| Recent messages | Bounded recent successful receptions and access to the station history inspector. |

Explain that more negative sensitivity values indicate reception of weaker
signals. Label outcome reasons as simulation diagnostics. Counters spanning an
RF edit show that they include earlier configurations. Display disabled channels
and zero opportunities distinctly from an upstream outage.

### Received-signal inspector

The table contains station, receive time, MMSI, message type, A/B channel,
estimated power dBm, margin dB, and transmission/reception IDs. Selecting a row
opens full raw checksummed NMEA with a copy control, decoded navigation, exact
virtual timestamps, model probability, distance at reception, and the RF settings
revision used for that decision. Render text with `textContent`, never raw HTML.

Support station and MMSI filters on the bounded loaded sample, labelled as sample
filters. History polling uses the selected station's independent cursor. Show
"History gap" with retained sequence bounds after missed events and reset the
loaded page as needed; do not erase current target observations. Keep at most 200
history rows in browser memory. Allow pausing that inspector's refresh so a user
can read a message while simulation polling continues. No historical export claim.

Failed decode attempts have no received AIS row. Aggregate reason counts explain
simulation misses without presenting hidden transmitter navigation as a reception.

### Target details

Show MMSI, received position, speed in knots, course/heading, last received UTC,
virtual age, state, chosen receiving station, and channel/power for that reception.
Show scenario name/type with a scenario label. Missing AIS values say unavailable.
Do not show unimplemented call sign, dimensions, or destination as received data.

Provide a receiver provenance table: station, last seen, chosen-transmission
match, channel, estimated power, and RF revision. A station with an older report
is labelled as such. Selecting its row switches to that station view and fetches
the corresponding last received position. Multiple receivers never create
duplicate aggregate targets or imply all sites heard the latest transmission.

### Loading, stale data, and empty states

| Situation | Display behavior |
| --- | --- |
| Initial load | Connecting status and stable empty panels; no fabricated station/target data. |
| Zero configured stations | "No base stations configured" and manager link; generation can still run. |
| Station receives nothing | Site and coverage remain visible; counters explain no opportunities, no active channels, or missed opportunities. |
| Target ages out | Remove at snapshot expiry; an open detail says the last-known target expired. |
| Station deleted during selection | Show removal status, switch to all current stations, and discard that station cursor. |
| Same run, RF edit | Replace changed geometry/configuration together; preserve historical message attribution and map zoom. |
| Simulator restart | Clear stations, targets, detail selection, inspector rows, and all cursors before drawing the new identity. |
| Upstream 502/503 | Keep last complete map, coverage, counters, and clock; mark the whole view stale and retry. |
| Paused simulation | Freeze target age and rates at committed virtual time; browser fetch status can continue updating. |
| Map tiles unavailable | Keep received data tables usable and report tile failure separately. |

Continue one main poll at a time, one real second after completion. Abort or ignore
outdated selection/detail requests using an in-browser request generation token.
Clear stale detail results whose run or selected station no longer matches.
DOM and marker updates use IDs; rebuild coverage only when RF revision changes.

Use keyboard-accessible buttons/tables, focus retention, status announcements,
adequate contrast, and station names alongside color. Avoid announcing every
high-frequency counter change through a live region.

## Verification

Run a browser checklist against deterministic fixtures for overlap, station-only
reception, no reception, channel impairment, lost targets, history gaps, name/RF
edits, removal, pause, restart, tile failure, and HTTP failure. Check mobile layout,
keyboard use, unknown AIS values, and HTML-like station names. Capture evidence
of the coverage map, station capability panel, target provenance, and raw message
inspector. Use existing Go handler/template tests for expected markup and data
contracts. Add small automated frontend checks only if supported by existing tools.

## Acceptance criteria

- Coverage, capabilities, station details, received signals, and AIS target details
  are accessible without leaving the display.
- Selecting a station visibly changes the observed target set and last-known data.
- Estimated RF values and scenario metadata are distinguishable from AIS payload data.
- Filters and map controls perform no simulation writes.
- Stale network data, stale AIS targets, no receivers, and no receptions have
  separate understandable states.

## Implementation record

Implemented in the embedded display template, `display.js`, and shared CSS.
The display remains a read-only consumer of its same-origin API; the engine
continues to own coverage geometry and reception decisions.

- All/single/multiple station selection, separate A/B coverage, layer controls,
  stable station colors, first-load framing, and explicit coverage fitting.
- Keyed comparison, target, provenance, and reception tables; station and target
  panels; original NMEA copying; nullable AIS fields labelled unavailable.
- Station history has an independent cursor and a 200-row bound plus one pinned
  message. Gaps retain current targets. Pause, removal, 409, new runs, and delayed
  replies preserve the appropriate identity boundaries.
- Main polling is serialized and waits one second after completion. Failed reads
  retain the complete previous view. Leaflet and tile failures leave tables usable.
- Standalone display links to its configured simulator's manager. No component
  shares simulation state or performs browser-side RF calculations.

The existing snapshot publishes per-station combined current and lost counts.
Fresh/stale splits and last-seen timestamps are derived only for selected sites
from retained observations, with this scope labelled beside the comparison.
Unselected sites show combined current totals and an unavailable last seen.
This avoids inventing missing lifetime timestamps or making extra upstream reads.
The model has published parameters rather than a version, consistent with the
proof-of-concept decision recorded in this plan's README. Optional target-to-site
lines are omitted; the provenance table identifies chosen and older transmissions.

Repeatable browser verification is in
`internal/ui/testdata/display-browser.mjs`, with invocation and evidence capture
in the adjacent README. It exercises deterministic received-data fixtures while
using the real embedded page, DOM, and Leaflet. Handler tests cover required
controls and combined/standalone manager links.

Verification on 2026-09-14: the browser checklist passed, including clipboard
CRLF preservation, delayed history and selection responses, paused virtual time,
restart with an explicit selection, and map dependency failures. Five screenshots
were captured using `AIS_BROWSER_EVIDENCE`. `task all` passed with 594 tests,
formatting, vet, dead-code, architecture, and lint checks.

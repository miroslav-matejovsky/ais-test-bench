# AIS Simulation

The `simulation` package is the traffic engine behind this test bench. It
generates synthetic AIS position reports for a fleet of vessels, models which
shore stations would actually receive each report, and exposes both as a
deterministic, replayable engine. The applications (`cmd/ais-test-bench`,
`cmd/simulator`, `cmd/display`) are one consumer of the engine; any Go program
can import `simulation` directly.

## What it supports

- **Fleet and movement.** 0-100 vessels (`simulation.MaxVessels`), each with a
  stable synthetic MMSI, name, vessel type, and a random speed and course.
  Vessels move at their reported speed and course over the earth's surface.
  Only one vessel category exists today, "Cargo vessel"; the catalog is an
  application-level concept (`Metadata.VesselTypes`), not a numeric AIS ship
  type field.
- **AIS messages.** Every vessel reports once per virtual second as a
  checksummed, single-fragment `!AIVDM` type 1 (Class A position report)
  sentence, alternating channels A and B per the field layout in the USCG
  type 1 documentation. Only type 1 is generated and decoded; other AIS
  message types are out of scope for now.
- **Virtual clock.** Simulation time runs independently of wall-clock time,
  from 0.01x to 100x real time, or paused. Speed changes how fast time
  passes, never the knots a vessel reports. The same config and ordered calls
  reproduce the same bytes.
- **Receiving stations.** 0-16 synthetic shore sites (`simulation.MaxStations`),
  each with position, antenna height, receive gain, feeder loss, per-channel
  sensitivity, and up to 8 bearing sectors with extra shadow loss. A
  deterministic link-budget model turns transmitter power, antenna heights,
  free-space path loss, and shadow loss into a per-station, per-channel
  receive probability, so which stations "hear" a given report is not
  binary and not uniform with distance.
- **Receptions and observations.** Every generated report is one reception
  opportunity at every configured station. Successful receptions build
  per-station counters, coverage contours (90%/50% probability rings), and a
  live set of observed targets built only from what was actually received -
  a vessel no station hears is not visible, and a missed report leaves the
  last known position stale.
- **History and APIs.** The latest 1,000 reports and the newest receptions
  per station are retained and queryable over HTTP (`internal/simulatorapi`)
  or in-process (`simulation.History`, `simulation.ReceptionHistory`). See
  the root [`README.md`](../README.md) for the full API and Go package
  reference.

## Rationale: why this models real-world AIS

AIS receiver, display, and fleet-monitoring software cannot be validated
against live traffic alone: live traffic is not reproducible, does not cover
edge cases on demand, and depends on hardware. The simulation targets the
specific real-world behaviors that make AIS software hard to get right:

- **Coverage is probabilistic, not a coastline.** A real VHF receive site
  does not have a hard reception radius; whether a message is decoded
  depends on transmitter power, antenna heights, distance, and terrain. The
  reception model (radio horizon, free-space loss, and a probability curve
  against receiver sensitivity) is built from public reference values - AIS
  channel frequencies (161.975/162.025 MHz, USCG), free-space loss per
  ITU-R P.525-5, the ITU-R M.1371-6 sensitivity test point, and IALA
  G1111-2's guidance that coverage depends on both antenna heights and
  installation losses - so that gaps, stale targets, and fading near the
  horizon behave like a real coastal AIS network, not a simple radius cutoff.
  The model's specific parameters are test-bench choices, not calibrated
  predictions; see `simulation.Metadata().Settings.Reception` and the
  package doc for exact sources and caveats.
- **Multiple stations disagree, like a real base station network.** The
  demonstration scenario (`internal/simdriver`) places three stations around
  Rotterdam's approach with different antenna heights and sensitivities, one
  with a shadow sector modeling a harbor obstruction. This mirrors how a
  Vessel Traffic Service (VTS) or AIS network operator combines several
  shore sites with overlapping, non-identical coverage to track vessels
  approaching a busy port - exactly the "union of stations" query the
  display and `Observations` API expose.
- **Channels fail independently, like real VHF interference.** Each station
  models channel A and B separately, with an optional drop probability per
  channel. The manager's "Disable B preset" exercises this through the same
  station API a real config change would use, letting client software be
  tested against a live channel outage or slot congestion instead of only
  the ideal case.
- **High-density traffic exposes gaps a client must handle.** Up to 100
  vessels at 100x speed emit up to 10,000 reports per real second. This
  is a stand-in for a busy port or strait (e.g. the Dover Strait or the
  Rotterdam approach) where message volume is high enough that a client
  polling a fixed-size history or slot-limited feed will legitimately miss
  reports. The bounded history, sequence numbers, and gap reporting in the
  API let client software be tested against exactly that failure mode
  instead of assuming every message arrives.
- **Time control supports both real-time and accelerated testing.** Running
  at 1x reproduces the pacing of watching a live feed; running faster
  compresses a multi-hour transit or a coverage-edge scenario into seconds
  for automated tests, without changing the physical speed vessels report.

## Known limitations

- Only one vessel type (cargo) and one AIS message type (type 1 position
  reports) are generated.
- Vessels start at random offshore positions with no coastline avoidance or
  route planning.
- Terrain, multipath, ducting, and slot collisions are not modeled; only the
  link-budget terms listed above are.
- No TCP/UDP publishing or playback of recorded traffic yet.

These are tracked as future design work in the root `README.md` and, where
concrete, in [`backlog/`](backlog/README.md).
